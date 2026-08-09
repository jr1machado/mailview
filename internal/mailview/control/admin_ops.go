package control

import (
	"context"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
)

// ResetTenantOwner grants newOwnerUserID the Tenant Owner role, creating a
// membership if the user is not already one. It does not remove the
// previous owner's membership or roles: ownership reassignment is an
// additive grant an operator can follow up by revoking the old owner's
// roles explicitly via ReplaceMembershipRoles, keeping this single action
// safe even if the target turns out wrong.
func (s *Service) ResetTenantOwner(ctx context.Context, tenantID uuid.UUID, newOwnerUserID int, actor Actor) (Membership, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Membership{}, err
	}
	defer tx.Rollback()

	if err := validateTenantAndUser(ctx, tx, tenantID, newOwnerUserID); err != nil {
		return Membership{}, err
	}

	var ownerRoleID uuid.UUID
	if err := tx.GetContext(ctx, &ownerRoleID, `SELECT id FROM mv_roles WHERE tenant_id = $1 AND name = 'Tenant Owner'`, tenantID); err != nil {
		if isNoRows(err) {
			return Membership{}, ErrNotFound
		}
		return Membership{}, err
	}

	var membership Membership
	err = tx.GetContext(ctx, &membership, `
INSERT INTO mv_memberships (id, tenant_id, user_id, status)
VALUES ($1, $2, $3, 'active')
ON CONFLICT (tenant_id, user_id) DO UPDATE SET status = 'active', updated_at = now()
RETURNING id, tenant_id, user_id, status, created_at, updated_at`, uuid.Must(uuid.NewV4()), tenantID, newOwnerUserID)
	if err != nil {
		return Membership{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_membership_roles (membership_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, membership.ID, ownerRoleID); err != nil {
		return Membership{}, err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "tenant.owner.reset", "tenant", tenantID.String(), "success", "", map[string]any{"new_owner_user_id": newOwnerUserID}); err != nil {
		return Membership{}, err
	}
	if err := tx.Commit(); err != nil {
		return Membership{}, err
	}
	// Ownership changed: force re-login same as any other sensitive role
	// change (Fase-4.md 10.4). Same query core.DeleteUserSessions uses.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE data->>'user_id' = $1`, strconv.Itoa(newOwnerUserID)); err != nil {
		return membership, err
	}
	return membership, nil
}

type TenantInfrastructure struct {
	TenantID    uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Mode        string    `db:"mode" json:"mode"`
	RequestedBy int       `db:"requested_by" json:"requested_by"`
	RequestedAt time.Time `db:"requested_at" json:"requested_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

var validInfrastructureModes = map[string]struct{}{"shared": {}, "dedicated_requested": {}, "dedicated": {}}

// SetTenantInfrastructureMode records the requested topology for a tenant
// (Fase-3.md 6.6 / Fase-4.md 11.2 "migrar para dedicado"). It only stores
// the flag: provisioning a dedicated database/worker/SMTP/namespace happens
// outside this codebase, driven by whoever reads this table.
func (s *Service) SetTenantInfrastructureMode(ctx context.Context, tenantID uuid.UUID, mode string, actor Actor) (TenantInfrastructure, error) {
	if _, ok := validInfrastructureModes[mode]; !ok {
		return TenantInfrastructure{}, ErrInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return TenantInfrastructure{}, err
	}
	defer tx.Rollback()

	var tenantExists bool
	if err := tx.GetContext(ctx, &tenantExists, `SELECT EXISTS(SELECT 1 FROM mv_tenants WHERE id = $1)`, tenantID); err != nil {
		return TenantInfrastructure{}, err
	}
	if !tenantExists {
		return TenantInfrastructure{}, ErrNotFound
	}

	var out TenantInfrastructure
	if err := tx.GetContext(ctx, &out, `
INSERT INTO mv_tenant_infrastructure (tenant_id, mode, requested_by)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id) DO UPDATE SET mode = EXCLUDED.mode, requested_by = EXCLUDED.requested_by, updated_at = now()
RETURNING tenant_id, mode, requested_by, requested_at, updated_at`, tenantID, mode, actor.UserID); err != nil {
		return TenantInfrastructure{}, err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "tenant.infrastructure.set_mode", "tenant", tenantID.String(), "success", "", map[string]any{"mode": mode}); err != nil {
		return TenantInfrastructure{}, err
	}
	return out, tx.Commit()
}

type PlatformDashboard struct {
	TenantsActive              int `db:"tenants_active" json:"tenants_active"`
	TenantsSuspended           int `db:"tenants_suspended" json:"tenants_suspended"`
	TenantsPending             int `db:"tenants_pending" json:"tenants_pending"`
	TenantsOffboarded          int `db:"tenants_offboarded" json:"tenants_offboarded"`
	DomainsPendingVerification int `db:"domains_pending_verification" json:"domains_pending_verification"`
	AuditEventsLast24h         int `db:"audit_events_last_24h" json:"audit_events_last_24h"`
	ImportJobsFailedLast24h    int `db:"import_jobs_failed_last_24h" json:"import_jobs_failed_last_24h"`
	ActiveImpersonationGrants  int `db:"active_impersonation_grants" json:"active_impersonation_grants"`
}

// GetPlatformDashboard aggregates MailView-owned metrics only. It
// deliberately excludes campaign/bounce/queue volume: those tables are not
// tenant-scoped yet (Fase-3.md 7.2 gap), so a platform-wide count would
// misrepresent itself as a tenant metric.
func (s *Service) GetPlatformDashboard(ctx context.Context) (PlatformDashboard, error) {
	var out PlatformDashboard
	err := s.db.GetContext(ctx, &out, `
SELECT
    (SELECT count(*) FROM mv_tenants WHERE status = 'active') AS tenants_active,
    (SELECT count(*) FROM mv_tenants WHERE status = 'suspended') AS tenants_suspended,
    (SELECT count(*) FROM mv_tenants WHERE status = 'pending') AS tenants_pending,
    (SELECT count(*) FROM mv_tenants WHERE status = 'offboarded') AS tenants_offboarded,
    (SELECT count(*) FROM mv_tenant_domains WHERE status = 'pending') AS domains_pending_verification,
    (SELECT count(*) FROM mv_audit_events WHERE occurred_at > now() - interval '24 hours') AS audit_events_last_24h,
    (SELECT count(*) FROM mv_import_jobs WHERE status = 'failed' AND updated_at > now() - interval '24 hours') AS import_jobs_failed_last_24h,
    (SELECT count(*) FROM mv_impersonation_grants WHERE revoked_at IS NULL AND expires_at > now()) AS active_impersonation_grants
`)
	return out, err
}
