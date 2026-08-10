package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	"github.com/lib/pq"
)

type APIKey struct {
	ID          uuid.UUID      `db:"id" json:"id"`
	Name        string         `db:"name" json:"name"`
	KeyPrefix   string         `db:"key_prefix" json:"key_prefix"`
	Permissions pq.StringArray `db:"permissions" json:"permissions"`
	LastUsedAt  *time.Time     `db:"last_used_at" json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time     `db:"expires_at" json:"expires_at,omitempty"`
	RevokedAt   *time.Time     `db:"revoked_at" json:"revoked_at,omitempty"`
	CreatedBy   int            `db:"created_by" json:"created_by"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
}

type CreateAPIKeyInput struct {
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type CreatedAPIKey struct {
	APIKey
	Token string `json:"token"`
}

// CreateAPIKey returns the token once and stores only its SHA-256 digest.
// Requested grants must be tenant-scoped and already effective for the actor.
func (s *Service) CreateAPIKey(ctx context.Context, in CreateAPIKeyInput, actor Actor) (CreatedAPIKey, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 100 || len(in.Permissions) == 0 || actor.UserID < 1 {
		return CreatedAPIKey{}, ErrInvalid
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		return CreatedAPIKey{}, ErrInvalid
	}
	scope, ok := tenant.FromContext(ctx)
	if !ok {
		return CreatedAPIKey{}, tenant.ErrMissingContext
	}
	seen := make(map[string]struct{}, len(in.Permissions))
	for _, code := range in.Permissions {
		if !strings.HasSuffix(code, ".tenant") || strings.Contains(code, ".platform") {
			return CreatedAPIKey{}, ErrInvalid
		}
		if _, duplicate := seen[code]; duplicate {
			return CreatedAPIKey{}, ErrInvalid
		}
		seen[code] = struct{}{}
		allowed, err := s.HasTenantPermission(ctx, scope.TenantID, actor.UserID, code)
		if err != nil {
			return CreatedAPIKey{}, err
		}
		if !allowed {
			return CreatedAPIKey{}, ErrInvalid
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return CreatedAPIKey{}, err
	}
	prefixBytes := make([]byte, 6)
	if _, err := rand.Read(prefixBytes); err != nil {
		return CreatedAPIKey{}, err
	}
	prefix := hex.EncodeToString(prefixBytes)
	token := "mv_" + prefix + "." + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))

	tx, _, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return CreatedAPIKey{}, err
	}
	defer tx.Rollback()
	out := CreatedAPIKey{APIKey: APIKey{ID: uuid.Must(uuid.NewV4()), Name: name, KeyPrefix: prefix, Permissions: pq.StringArray(in.Permissions), ExpiresAt: in.ExpiresAt, CreatedBy: actor.UserID}, Token: token}
	if err := tx.GetContext(ctx, &out.APIKey, `INSERT INTO mv_api_keys
 (id,tenant_id,name,key_prefix,secret_hash,permissions,expires_at,created_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)
 RETURNING id,name,key_prefix,permissions,last_used_at,expires_at,revoked_at,created_by,created_at`,
		out.ID, scope.TenantID, name, prefix, hex.EncodeToString(digest[:]), pq.Array(in.Permissions), in.ExpiresAt, actor.UserID); err != nil {
		if isUniqueViolation(err) {
			return CreatedAPIKey{}, ErrConflict
		}
		return CreatedAPIKey{}, err
	}
	if err := appendAudit(ctx, tx, &scope.TenantID, actor, "api_key.create", "api_key", out.ID.String(), "success", "", map[string]any{"name": name, "key_prefix": prefix}); err != nil {
		return CreatedAPIKey{}, err
	}
	return out, tx.Commit()
}

func (s *Service) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	tx, _, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := []APIKey{}
	if err := tx.SelectContext(ctx, &out, `SELECT id,name,key_prefix,permissions,last_used_at,expires_at,revoked_at,created_by,created_at FROM mv_api_keys ORDER BY created_at DESC`); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

func (s *Service) RevokeAPIKey(ctx context.Context, id uuid.UUID, actor Actor) error {
	tx, scope, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE mv_api_keys SET revoked_at=now() WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL`, scope.TenantID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := appendAudit(ctx, tx, &scope.TenantID, actor, "api_key.revoke", "api_key", id.String(), "success", "", nil); err != nil {
		return err
	}
	return tx.Commit()
}
