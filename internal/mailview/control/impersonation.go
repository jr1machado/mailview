package control

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

var (
	ErrMFANotRecent  = errors.New("actor must have verified TOTP recently to start impersonation")
	ErrGrantExpired  = errors.New("impersonation grant has expired or was revoked")
	ErrGrantNotOwned = errors.New("impersonation grant does not belong to the calling user")
)

// mfaRecencyWindow bounds how long ago the actor's last successful TOTP
// verification (control.MFA.MarkUsed) may have happened. Fase-4.md 11.3
// requires "MFA recente"; this is intentionally short since a grant itself
// is also short-lived.
const mfaRecencyWindow = 15 * time.Minute

const (
	maxImpersonationTTL = 30 * time.Minute
	minReasonLength     = 10
)

type ImpersonationGrant struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	TenantID     uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	ActorUserID  int        `db:"actor_user_id" json:"actor_user_id"`
	TargetUserID int        `db:"target_user_id" json:"target_user_id"`
	Reason       string     `db:"reason" json:"reason"`
	ApprovedBy   *int       `db:"approved_by" json:"approved_by,omitempty"`
	ExpiresAt    time.Time  `db:"expires_at" json:"expires_at"`
	RevokedAt    *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

type StartImpersonationInput struct {
	TenantID     uuid.UUID  `json:"tenant_id"`
	TargetUserID int        `json:"target_user_id"`
	Reason       string     `json:"reason"`
	TTLMinutes   int        `json:"ttl_minutes"`
	ApprovedBy   *int       `json:"approved_by,omitempty"`
}

// StartImpersonation opens a time-boxed, tenant-data-scoped grant letting
// actor act as target for support purposes. It enforces every hard
// requirement from Fase-4.md 11.3 except the visible banner and secret/
// billing exclusions, which are enforced by what the grant is (and is not)
// wired into: it only ever widens access to the tenant data plane
// (subscribers/lists), never to platform permissions, MFA secrets or
// recovery codes, or billing endpoints, because nothing checks this grant
// there.
func (s *Service) StartImpersonation(ctx context.Context, actorUserID int, in StartImpersonationInput, actor Actor) (ImpersonationGrant, error) {
	reason := strings.TrimSpace(in.Reason)
	if len(reason) < minReasonLength {
		return ImpersonationGrant{}, ErrInvalid
	}
	if in.TargetUserID < 1 || in.TargetUserID == actorUserID {
		return ImpersonationGrant{}, ErrInvalid
	}
	ttl := time.Duration(in.TTLMinutes) * time.Minute
	if ttl <= 0 || ttl > maxImpersonationTTL {
		return ImpersonationGrant{}, ErrInvalid
	}

	var lastVerified sql.NullTime
	if err := s.db.GetContext(ctx, &lastVerified, `SELECT last_used_at FROM mv_mfa_methods WHERE user_id = $1 AND type = 'totp'`, actorUserID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ImpersonationGrant{}, err
	}
	if !lastVerified.Valid || time.Since(lastVerified.Time) > mfaRecencyWindow {
		return ImpersonationGrant{}, ErrMFANotRecent
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return ImpersonationGrant{}, err
	}
	defer tx.Rollback()

	var isMember bool
	if err := tx.GetContext(ctx, &isMember, `SELECT EXISTS(SELECT 1 FROM mv_memberships WHERE tenant_id = $1 AND user_id = $2 AND status = 'active')`, in.TenantID, in.TargetUserID); err != nil {
		return ImpersonationGrant{}, err
	}
	if !isMember {
		return ImpersonationGrant{}, ErrNotFound
	}

	out := ImpersonationGrant{
		ID: uuid.Must(uuid.NewV4()), TenantID: in.TenantID, ActorUserID: actorUserID, TargetUserID: in.TargetUserID,
		Reason: reason, ApprovedBy: in.ApprovedBy,
	}
	if err := tx.GetContext(ctx, &out, `
INSERT INTO mv_impersonation_grants (id, tenant_id, actor_user_id, target_user_id, reason, approved_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
RETURNING id, tenant_id, actor_user_id, target_user_id, reason, approved_by, expires_at, revoked_at, created_at`,
		out.ID, out.TenantID, out.ActorUserID, out.TargetUserID, out.Reason, out.ApprovedBy, ttl.String()); err != nil {
		return ImpersonationGrant{}, err
	}
	if err := appendAudit(ctx, tx, &in.TenantID, actor, "impersonation.start", "user", strconv.Itoa(in.TargetUserID), "success", reason, map[string]any{"grant_id": out.ID.String(), "ttl_minutes": in.TTLMinutes}); err != nil {
		return ImpersonationGrant{}, err
	}
	return out, tx.Commit()
}

// RevokeImpersonation ends a grant early. Anyone with the platform
// permission that started grants may revoke any grant, not only their own,
// so a second responder can cut off a session that looks wrong.
func (s *Service) RevokeImpersonation(ctx context.Context, grantID uuid.UUID, actor Actor) (ImpersonationGrant, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return ImpersonationGrant{}, err
	}
	defer tx.Rollback()

	var out ImpersonationGrant
	if err := tx.GetContext(ctx, &out, `
UPDATE mv_impersonation_grants SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL
RETURNING id, tenant_id, actor_user_id, target_user_id, reason, approved_by, expires_at, revoked_at, created_at`, grantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ImpersonationGrant{}, ErrNotFound
		}
		return ImpersonationGrant{}, err
	}
	if err := appendAudit(ctx, tx, &out.TenantID, actor, "impersonation.revoke", "user", strconv.Itoa(out.TargetUserID), "success", "", map[string]any{"grant_id": out.ID.String()}); err != nil {
		return ImpersonationGrant{}, err
	}
	return out, tx.Commit()
}

// ListImpersonationGrants returns every grant, most recent first, for the
// platform audit view. There is no per-tenant filter here on purpose: the
// whole point of this list is a platform-wide "who is acting as whom".
func (s *Service) ListImpersonationGrants(ctx context.Context) ([]ImpersonationGrant, error) {
	out := []ImpersonationGrant{}
	err := s.db.SelectContext(ctx, &out, `
SELECT id, tenant_id, actor_user_id, target_user_id, reason, approved_by, expires_at, revoked_at, created_at
FROM mv_impersonation_grants ORDER BY created_at DESC LIMIT 200`)
	return out, err
}

// ValidateImpersonationGrant checks that grantID is active (not expired,
// not revoked) and was opened by actorUserID, returning the target user ID
// to scope the request as. Callers must not use this to bypass anything
// other than "which user_id does this tenant-scoped request act as".
func (s *Service) ValidateImpersonationGrant(ctx context.Context, grantID uuid.UUID, actorUserID int) (ImpersonationGrant, error) {
	var out ImpersonationGrant
	err := s.db.GetContext(ctx, &out, `
SELECT id, tenant_id, actor_user_id, target_user_id, reason, approved_by, expires_at, revoked_at, created_at
FROM mv_impersonation_grants WHERE id = $1`, grantID)
	if errors.Is(err, sql.ErrNoRows) {
		return ImpersonationGrant{}, ErrNotFound
	}
	if err != nil {
		return ImpersonationGrant{}, err
	}
	if out.ActorUserID != actorUserID {
		return ImpersonationGrant{}, ErrGrantNotOwned
	}
	if out.RevokedAt != nil || time.Now().After(out.ExpiresAt) {
		return ImpersonationGrant{}, ErrGrantExpired
	}
	return out, nil
}
