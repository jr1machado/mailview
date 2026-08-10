// Package importjob implements tenant-scoped CSV subscriber imports. Each
// job's file is written under a per-tenant storage prefix and signed with an
// HMAC so a worker can detect a swapped or corrupted upload before it ever
// touches another tenant's data.
package importjob

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	mvjob "github.com/knadh/listmonk/internal/mailview/job"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	"github.com/lib/pq"
)

var (
	ErrInvalid            = errors.New("invalid import job input")
	ErrNotFound           = errors.New("import job not found")
	ErrSigningUnavailable = errors.New("import signing key is not configured")
	errJobCancelled       = errors.New("import job cancelled")

	batchSize = 500
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

type Job struct {
	ID            uuid.UUID     `db:"id" json:"id"`
	Status        string        `db:"status" json:"status"`
	ListIDs       pq.Int64Array `db:"list_ids" json:"list_ids"`
	TotalRows     int           `db:"total_rows" json:"total_rows"`
	ProcessedRows int           `db:"processed_rows" json:"processed_rows"`
	ImportedRows  int           `db:"imported_rows" json:"imported_rows"`
	ErrorRows     int           `db:"error_rows" json:"error_rows"`
	Error         string        `db:"error" json:"error,omitempty"`
}

type CreateJobInput struct {
	IdempotencyKey string
	ListIDs        []int
}

// Service creates and runs tenant-scoped CSV import jobs. Files are written
// under storageDir/<tenant_id>/ so two tenants can never share a path.
type Service struct {
	db         *sqlx.DB
	storageDir string
	signingKey []byte
	jobSigner  *mvjob.Signer
}

// New builds the import job service. encodedKey is a base64-encoded 32-byte
// HMAC key; an empty key disables job creation until one is configured,
// mirroring how mvcontrol.NewMFA gates TOTP enrollment.
func New(db *sqlx.DB, storageDir, encodedKey string) (*Service, error) {
	s := &Service{db: db, storageDir: storageDir}
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return s, nil
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("decode MailView import signing key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("MailView import signing key must decode to 32 bytes")
	}
	s.signingKey = key
	s.jobSigner, err = mvjob.NewSigner(encodedKey)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// WorkerEnvelope signs tenant_id together with the job identity. It is the
// only payload accepted by detached/asynchronous import workers.
func (s *Service) WorkerEnvelope(ctx context.Context, jobID uuid.UUID) (mvjob.Envelope, error) {
	if s.jobSigner == nil {
		return mvjob.Envelope{}, ErrSigningUnavailable
	}
	scope, ok := tenant.FromContext(ctx)
	if !ok {
		return mvjob.Envelope{}, tenant.ErrMissingContext
	}
	return s.jobSigner.Sign(jobID, scope.TenantID, "subscriber_import.process", map[string]string{"job_id": jobID.String()}, 24*time.Hour)
}

func (s *Service) ProcessEnvelope(ctx context.Context, envelope mvjob.Envelope) error {
	if s.jobSigner == nil {
		return ErrSigningUnavailable
	}
	tenantID, err := s.jobSigner.Verify(envelope)
	if err != nil || envelope.Type != "subscriber_import.process" || envelope.JobID == uuid.Nil {
		return ErrInvalid
	}
	scoped := tenant.WithContext(ctx, tenant.Context{TenantID: tenantID, RequestID: "job:" + envelope.JobID.String()})
	return s.ProcessJob(scoped, envelope.JobID)
}

// CreateJob validates that every requested list belongs to the caller's
// tenant, persists the upload to an isolated path, and records the job.
// Replaying the same idempotency key returns the original job instead of
// creating a duplicate.
func (s *Service) CreateJob(ctx context.Context, in CreateJobInput, file io.Reader) (Job, error) {
	if len(s.signingKey) == 0 {
		return Job{}, ErrSigningUnavailable
	}
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 {
		return Job{}, ErrInvalid
	}

	var job Job
	var storagePath string
	var payload []byte
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		var existingID uuid.UUID
		lookupErr := tx.GetContext(ctx, &existingID, `SELECT id FROM mv_import_jobs WHERE tenant_id=$1 AND idempotency_key=$2`, scope.TenantID, in.IdempotencyKey)
		switch {
		case lookupErr == nil:
			return tx.GetContext(ctx, &job, `SELECT id,status,list_ids,total_rows,processed_rows,imported_rows,error_rows,error FROM mv_import_jobs WHERE tenant_id=$1 AND id=$2`, scope.TenantID, existingID)
		case !errors.Is(lookupErr, sql.ErrNoRows):
			return lookupErr
		}

		if len(in.ListIDs) > 0 {
			var count int
			if err := tx.GetContext(ctx, &count, `SELECT count(*) FROM lists WHERE tenant_id=$1 AND id=ANY($2)`, scope.TenantID, pq.Array(listIDsParam(in.ListIDs))); err != nil {
				return err
			}
			if count != len(in.ListIDs) {
				return fmt.Errorf("%w: list not found in tenant", ErrInvalid)
			}
		}

		data, err := io.ReadAll(io.LimitReader(file, 64<<20))
		if err != nil {
			return err
		}
		payload = data

		id := uuid.Must(uuid.NewV4())
		signature := s.sign(scope.TenantID.String(), id.String(), data)
		storagePath = filepath.Join(s.storageDir, scope.TenantID.String(), id.String()+".csv")

		if err := tx.GetContext(ctx, &job, `
INSERT INTO mv_import_jobs (id, tenant_id, actor_id, idempotency_key, list_ids)
VALUES ($1,$2,$3,$4,$5)
RETURNING id,status,list_ids,total_rows,processed_rows,imported_rows,error_rows,error`,
			id, scope.TenantID, scope.UserID, in.IdempotencyKey, pq.Array(listIDsParam(in.ListIDs))); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO mv_import_files (id, tenant_id, job_id, storage_key, signature, size_bytes) VALUES ($1,$2,$3,$4,$5,$6)`,
			uuid.Must(uuid.NewV4()), scope.TenantID, id, storagePath, signature, len(data))
		return err
	})
	if err != nil || storagePath == "" {
		return job, err
	}

	if err := os.MkdirAll(filepath.Dir(storagePath), 0o750); err != nil {
		return job, err
	}
	if err := os.WriteFile(storagePath, payload, 0o640); err != nil {
		return job, err
	}
	return job, nil
}

func (s *Service) GetJob(ctx context.Context, id uuid.UUID) (Job, error) {
	var out Job
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		e := tx.GetContext(ctx, &out, `SELECT id,status,list_ids,total_rows,processed_rows,imported_rows,error_rows,error FROM mv_import_jobs WHERE tenant_id=$1 AND id=$2`, scope.TenantID, id)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrNotFound
		}
		return e
	})
	return out, err
}

func (s *Service) ListJobs(ctx context.Context) ([]Job, error) {
	var out []Job
	err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		return tx.SelectContext(ctx, &out, `SELECT id,status,list_ids,total_rows,processed_rows,imported_rows,error_rows,error FROM mv_import_jobs WHERE tenant_id=$1 ORDER BY created_at DESC`, scope.TenantID)
	})
	return out, err
}

func (s *Service) CancelJob(ctx context.Context, id uuid.UUID) error {
	return tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		r, err := tx.ExecContext(ctx, `UPDATE mv_import_jobs SET status='cancelled', updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status IN ('pending','processing')`, scope.TenantID, id)
		if err != nil {
			return err
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return ErrNotFound
		}
		return nil
	})
}

// ProcessJob runs the CSV worker for a job. It re-validates the file
// signature and list ownership inside the tenant's own transaction context,
// so a job can never be processed against the wrong tenant even if its ID
// leaked. Rows are committed in batches so a large file can't hold one
// transaction open indefinitely.
func (s *Service) ProcessJob(ctx context.Context, jobID uuid.UUID) error {
	if len(s.signingKey) == 0 {
		return ErrSigningUnavailable
	}
	tctx, ok := tenant.FromContext(ctx)
	if !ok {
		return tenant.ErrMissingContext
	}

	var storageKey, signature string
	var listIDs []int64
	if err := tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		var job Job
		if err := tx.GetContext(ctx, &job, `SELECT id,status,list_ids FROM mv_import_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, scope.TenantID, jobID); err != nil {
			return err
		}
		if job.Status != StatusPending {
			return fmt.Errorf("%w: job is not pending", ErrInvalid)
		}
		listIDs = []int64(job.ListIDs)
		var file struct {
			StorageKey string `db:"storage_key"`
			Signature  string `db:"signature"`
		}
		if err := tx.GetContext(ctx, &file, `SELECT storage_key, signature FROM mv_import_files WHERE tenant_id=$1 AND job_id=$2`, scope.TenantID, jobID); err != nil {
			return err
		}
		storageKey, signature = file.StorageKey, file.Signature
		_, err := tx.ExecContext(ctx, `UPDATE mv_import_jobs SET status='processing', updated_at=now() WHERE tenant_id=$1 AND id=$2`, scope.TenantID, jobID)
		return err
	}); err != nil {
		return err
	}

	data, err := os.ReadFile(storageKey)
	if err != nil {
		s.fail(ctx, jobID, err)
		return err
	}
	if s.sign(tctx.TenantID.String(), jobID.String(), data) != signature {
		err := errors.New("import file signature mismatch")
		s.fail(ctx, jobID, err)
		return err
	}

	total, imported, errored, procErr := s.importRows(ctx, jobID, data, listIDs)
	if procErr != nil {
		if errors.Is(procErr, errJobCancelled) {
			return nil
		}
		s.fail(ctx, jobID, procErr)
		return procErr
	}

	return tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		_, err := tx.ExecContext(ctx, `UPDATE mv_import_jobs SET status='completed', total_rows=$3, processed_rows=$3, imported_rows=$4, error_rows=$5, updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='processing'`,
			scope.TenantID, jobID, total, imported, errored)
		return err
	})
}

func (s *Service) importRows(ctx context.Context, jobID uuid.UUID, data []byte, listIDs []int64) (total, imported, errored int, err error) {
	r := csv.NewReader(bytes.NewReader(data))
	header, err := r.Read()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read csv header: %w", err)
	}
	emailCol, nameCol := -1, -1
	for i, col := range header {
		switch strings.ToLower(strings.TrimSpace(col)) {
		case "email":
			emailCol = i
		case "name":
			nameCol = i
		}
	}
	if emailCol == -1 {
		return 0, 0, 0, fmt.Errorf("%w: csv is missing an email column", ErrInvalid)
	}

	var batch [][2]string
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		batchLen := len(batch)
		n, e := s.importBatch(ctx, jobID, batch, listIDs)
		imported += n
		errored += batchLen - n
		batch = batch[:0]
		return e
	}
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return total, imported, errored, fmt.Errorf("read csv row: %w", e)
		}
		total++
		if emailCol >= len(row) {
			errored++
			continue
		}
		email := strings.TrimSpace(row[emailCol])
		name := ""
		if nameCol != -1 && nameCol < len(row) {
			name = strings.TrimSpace(row[nameCol])
		}
		batch = append(batch, [2]string{email, name})
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return total, imported, errored, err
			}
		}
	}
	if err := flush(); err != nil {
		return total, imported, errored, err
	}
	return total, imported, errored, nil
}

func (s *Service) importBatch(ctx context.Context, jobID uuid.UUID, rows [][2]string, listIDs []int64) (imported int, err error) {
	err = tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		var status string
		if err := tx.GetContext(ctx, &status, `SELECT status FROM mv_import_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, scope.TenantID, jobID); err != nil {
			return err
		}
		if status == StatusCancelled {
			return errJobCancelled
		}
		if status != StatusProcessing {
			return fmt.Errorf("%w: job is not processing", ErrInvalid)
		}
		for _, row := range rows {
			email, name := row[0], row[1]
			if !strings.Contains(email, "@") {
				continue
			}
			var subID int
			if e := tx.GetContext(ctx, &subID, `
INSERT INTO subscribers (tenant_id, uuid, email, name, status)
VALUES ($1,$2,$3,$4,'enabled')
ON CONFLICT (tenant_id, lower(email)) DO UPDATE SET name = EXCLUDED.name, updated_at = now()
RETURNING id`, scope.TenantID, uuid.Must(uuid.NewV4()), strings.ToLower(email), name); e != nil {
				return e
			}
			for _, listID := range listIDs {
				if _, e := tx.ExecContext(ctx, `INSERT INTO subscriber_lists (tenant_id,subscriber_id,list_id,status) SELECT $1,$2,id,'unconfirmed' FROM lists WHERE id=$3 AND tenant_id=$1 ON CONFLICT DO NOTHING`, scope.TenantID, subID, listID); e != nil {
					return e
				}
			}
			imported++
		}
		_, err := tx.ExecContext(ctx, `UPDATE mv_import_jobs SET processed_rows=processed_rows+$3, imported_rows=imported_rows+$4, error_rows=error_rows+$5, updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='processing'`,
			scope.TenantID, jobID, len(rows), imported, len(rows)-imported)
		return err
	})
	return imported, err
}

func (s *Service) fail(ctx context.Context, jobID uuid.UUID, cause error) {
	_ = tenant.InTransaction(ctx, s.db, func(tx *sqlx.Tx, scope tenant.Context) error {
		_, err := tx.ExecContext(ctx, `UPDATE mv_import_jobs SET status='failed', error=$3, updated_at=now() WHERE tenant_id=$1 AND id=$2`, scope.TenantID, jobID, cause.Error())
		return err
	})
}

func (s *Service) sign(tenantID, jobID string, data []byte) string {
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(tenantID))
	mac.Write([]byte(jobID))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func listIDsParam(ids []int) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}
