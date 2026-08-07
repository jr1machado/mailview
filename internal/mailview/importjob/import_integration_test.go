package importjob

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/mailview/migrations"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	_ "github.com/lib/pq"
)

// TestConcurrentTenantImportsIntegration is opt-in and runs against the same
// temporary PostgreSQL used by the other MailView integration tests. It
// proves that two tenants importing at the same time never see each other's
// subscribers, and that replaying an idempotency key does not duplicate rows.
func TestConcurrentTenantImportsIntegration(t *testing.T) {
	dsn := os.Getenv("MAILVIEW_TEST_DSN")
	if dsn == "" {
		t.Skip("MAILVIEW_TEST_DSN is not set")
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := migrations.Upgrade(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO roles (id, type, name, permissions) VALUES (1, 'user', 'Super Admin', '{}') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_login, password, email, name, type, user_role_id, status) VALUES (1, 'owner', true, 'unused', 'owner@example.test', 'Owner', 'user', 1, 'enabled') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}

	a, b := uuid.Must(uuid.NewV4()), uuid.Must(uuid.NewV4())
	if _, err := db.ExecContext(ctx, `INSERT INTO mv_tenants (id,slug,name) VALUES ($1,'import-tenant-a','Import A'),($2,'import-tenant-b','Import B')`, a, b); err != nil {
		t.Fatal(err)
	}
	var listA, listB int
	if err := db.GetContext(ctx, &listA, `INSERT INTO lists (tenant_id,uuid,name,type,optin,status) VALUES ($1,$2,'A list','private','single','active') RETURNING id`, a, uuid.Must(uuid.NewV4())); err != nil {
		t.Fatal(err)
	}
	if err := db.GetContext(ctx, &listB, `INSERT INTO lists (tenant_id,uuid,name,type,optin,status) VALUES ($1,$2,'B list','private','single','active') RETURNING id`, b, uuid.Must(uuid.NewV4())); err != nil {
		t.Fatal(err)
	}

	key := make([]byte, 32)
	svc, err := New(db, t.TempDir(), base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}

	ctxA := tenant.WithContext(ctx, tenant.Context{TenantID: a, UserID: 1})
	ctxB := tenant.WithContext(ctx, tenant.Context{TenantID: b, UserID: 1})

	// A tenant may not attach an import to another tenant's list.
	if _, err := svc.CreateJob(ctxA, CreateJobInput{IdempotencyKey: "cross", ListIDs: []int{listB}}, strings.NewReader("email,name\nx@example.test,X\n")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected cross-tenant list attachment to be rejected, got %v", err)
	}

	csvA := "email,name\na1@example.test,A One\na2@example.test,A Two\n"
	csvB := "email,name\nb1@example.test,B One\n"

	var wg sync.WaitGroup
	jobs := make([]Job, 2)
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		jobs[0], errs[0] = svc.CreateJob(ctxA, CreateJobInput{IdempotencyKey: "job-a", ListIDs: []int{listA}}, strings.NewReader(csvA))
	}()
	go func() {
		defer wg.Done()
		jobs[1], errs[1] = svc.CreateJob(ctxB, CreateJobInput{IdempotencyKey: "job-b", ListIDs: []int{listB}}, strings.NewReader(csvB))
	}()
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("create job %d: %v", i, e)
		}
	}

	if err := svc.ProcessJob(ctxA, jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessJob(ctxB, jobs[1].ID); err != nil {
		t.Fatal(err)
	}

	jobA, err := svc.GetJob(ctxA, jobs[0].ID)
	if err != nil || jobA.Status != StatusCompleted || jobA.ImportedRows != 2 {
		t.Fatalf("tenant A job=%#v err=%v", jobA, err)
	}
	jobB, err := svc.GetJob(ctxB, jobs[1].ID)
	if err != nil || jobB.Status != StatusCompleted || jobB.ImportedRows != 1 {
		t.Fatalf("tenant B job=%#v err=%v", jobB, err)
	}

	var countA, countB int
	if err := db.GetContext(ctx, &countA, `SELECT count(*) FROM subscribers WHERE tenant_id=$1`, a); err != nil || countA != 2 {
		t.Fatalf("tenant A subscribers=%d err=%v", countA, err)
	}
	if err := db.GetContext(ctx, &countB, `SELECT count(*) FROM subscribers WHERE tenant_id=$1`, b); err != nil || countB != 1 {
		t.Fatalf("tenant B subscribers=%d err=%v", countB, err)
	}
	var crossLeak int
	if err := db.GetContext(ctx, &crossLeak, `SELECT count(*) FROM subscribers WHERE tenant_id=$1 AND email='b1@example.test'`, a); err != nil || crossLeak != 0 {
		t.Fatalf("tenant A leaked tenant B subscriber: count=%d err=%v", crossLeak, err)
	}

	// Replaying the idempotency key must return the same job, not a new one.
	replay, err := svc.CreateJob(ctxA, CreateJobInput{IdempotencyKey: "job-a", ListIDs: []int{listA}}, strings.NewReader(csvA))
	if err != nil || replay.ID != jobA.ID {
		t.Fatalf("idempotency replay=%#v err=%v", replay, err)
	}
	var jobCountA int
	if err := db.GetContext(ctx, &jobCountA, `SELECT count(*) FROM mv_import_jobs WHERE tenant_id=$1`, a); err != nil || jobCountA != 1 {
		t.Fatalf("tenant A job rows=%d err=%v", jobCountA, err)
	}
}

