package control

import (
	"context"
	"strconv"
	"strings"
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
	TenantID         uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	Mode             string     `db:"mode" json:"mode"`
	DatabaseRef      string     `db:"database_ref" json:"database_ref,omitempty"`
	WorkerRef        string     `db:"worker_ref" json:"worker_ref,omitempty"`
	SMTPRef          string     `db:"smtp_ref" json:"smtp_ref,omitempty"`
	StorageRef       string     `db:"storage_ref" json:"storage_ref,omitempty"`
	EncryptionKeyRef string     `db:"encryption_key_ref" json:"encryption_key_ref,omitempty"`
	DockerNamespace  string     `db:"docker_namespace" json:"docker_namespace,omitempty"`
	RoutingVersion   int64      `db:"routing_version" json:"routing_version"`
	RequestedBy      int        `db:"requested_by" json:"requested_by"`
	RequestedAt      time.Time  `db:"requested_at" json:"requested_at"`
	ActivatedAt      *time.Time `db:"activated_at" json:"activated_at,omitempty"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
}

type TenantInfrastructureInput struct {
	Mode             string `json:"mode"`
	DatabaseRef      string `json:"database_ref"`
	WorkerRef        string `json:"worker_ref"`
	SMTPRef          string `json:"smtp_ref"`
	StorageRef       string `json:"storage_ref"`
	EncryptionKeyRef string `json:"encryption_key_ref"`
	DockerNamespace  string `json:"docker_namespace"`
}

var validInfrastructureModes = map[string]struct{}{"shared": {}, "dedicated_requested": {}, "dedicated": {}}

func (s *Service) SetTenantInfrastructureMode(ctx context.Context, tenantID uuid.UUID, mode string, actor Actor) (TenantInfrastructure, error) {
	return s.ConfigureTenantInfrastructure(ctx, tenantID, TenantInfrastructureInput{Mode: mode}, actor)
}

// ConfigureTenantInfrastructure is the Control Plane routing contract for an
// Enterprise tenant. Values are secret/resource references, never credentials.
// A dedicated route cannot be activated until every isolation dimension has
// been provisioned explicitly.
func (s *Service) ConfigureTenantInfrastructure(ctx context.Context, tenantID uuid.UUID, in TenantInfrastructureInput, actor Actor) (TenantInfrastructure, error) {
	in.Mode = strings.TrimSpace(in.Mode)
	if _, ok := validInfrastructureModes[in.Mode]; !ok {
		return TenantInfrastructure{}, ErrInvalid
	}
	in.DatabaseRef = strings.TrimSpace(in.DatabaseRef)
	in.WorkerRef = strings.TrimSpace(in.WorkerRef)
	in.SMTPRef = strings.TrimSpace(in.SMTPRef)
	in.StorageRef = strings.TrimSpace(in.StorageRef)
	in.EncryptionKeyRef = strings.TrimSpace(in.EncryptionKeyRef)
	in.DockerNamespace = strings.TrimSpace(in.DockerNamespace)
	if in.Mode == "dedicated" && (in.DatabaseRef == "" || in.WorkerRef == "" || in.SMTPRef == "" || in.StorageRef == "" || in.EncryptionKeyRef == "" || in.DockerNamespace == "") {
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
INSERT INTO mv_tenant_infrastructure
 (tenant_id,mode,database_ref,worker_ref,smtp_ref,storage_ref,encryption_key_ref,docker_namespace,requested_by,activated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,CASE WHEN $2='dedicated' THEN now() ELSE NULL END)
ON CONFLICT (tenant_id) DO UPDATE SET
 mode=EXCLUDED.mode,database_ref=EXCLUDED.database_ref,worker_ref=EXCLUDED.worker_ref,smtp_ref=EXCLUDED.smtp_ref,
 storage_ref=EXCLUDED.storage_ref,encryption_key_ref=EXCLUDED.encryption_key_ref,docker_namespace=EXCLUDED.docker_namespace,
 requested_by=EXCLUDED.requested_by,routing_version=mv_tenant_infrastructure.routing_version+1,
 activated_at=CASE WHEN EXCLUDED.mode='dedicated' THEN COALESCE(mv_tenant_infrastructure.activated_at,now()) ELSE NULL END,updated_at=now()
RETURNING tenant_id,mode,database_ref,worker_ref,smtp_ref,storage_ref,encryption_key_ref,docker_namespace,
 routing_version,requested_by,requested_at,activated_at,updated_at`, tenantID, in.Mode, in.DatabaseRef, in.WorkerRef, in.SMTPRef, in.StorageRef, in.EncryptionKeyRef, in.DockerNamespace, actor.UserID); err != nil {
		return TenantInfrastructure{}, err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "tenant.infrastructure.configure", "tenant", tenantID.String(), "success", "", map[string]any{"mode": in.Mode, "routing_version": out.RoutingVersion}); err != nil {
		return TenantInfrastructure{}, err
	}
	return out, tx.Commit()
}

// ResolveTenantInfrastructure returns an immutable routing decision for
// gateways/workers. Shared is the safe default for tenants without a row.
func (s *Service) ResolveTenantInfrastructure(ctx context.Context, tenantID uuid.UUID) (TenantInfrastructure, error) {
	var out TenantInfrastructure
	err := s.db.GetContext(ctx, &out, `SELECT tenant_id,mode,database_ref,worker_ref,smtp_ref,storage_ref,encryption_key_ref,docker_namespace,
		routing_version,requested_by,requested_at,activated_at,updated_at FROM mv_tenant_infrastructure WHERE tenant_id=$1`, tenantID)
	if err == nil {
		return out, nil
	}
	if !isNoRows(err) {
		return TenantInfrastructure{}, err
	}
	var exists bool
	if err := s.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM mv_tenants WHERE id=$1)`, tenantID); err != nil {
		return TenantInfrastructure{}, err
	}
	if !exists {
		return TenantInfrastructure{}, ErrNotFound
	}
	return TenantInfrastructure{TenantID: tenantID, Mode: "shared", RoutingVersion: 0}, nil
}

type PlatformDashboard struct {
	TenantsActive              int   `db:"tenants_active" json:"tenants_active"`
	TenantsSuspended           int   `db:"tenants_suspended" json:"tenants_suspended"`
	TenantsPending             int   `db:"tenants_pending" json:"tenants_pending"`
	TenantsOffboarded          int   `db:"tenants_offboarded" json:"tenants_offboarded"`
	DomainsPendingVerification int   `db:"domains_pending_verification" json:"domains_pending_verification"`
	AuditEventsLast24h         int   `db:"audit_events_last_24h" json:"audit_events_last_24h"`
	ImportJobsFailedLast24h    int   `db:"import_jobs_failed_last_24h" json:"import_jobs_failed_last_24h"`
	ActiveImpersonationGrants  int   `db:"active_impersonation_grants" json:"active_impersonation_grants"`
	MRRCents                   int64 `db:"mrr_cents" json:"mrr_cents"`
	ARRCents                   int64 `db:"arr_cents" json:"arr_cents"`
	EmailsSentThisMonth        int64 `json:"emails_sent_this_month"`
	QueuedMessages             int   `json:"queued_messages"`
	BouncesLast24h             int   `json:"bounces_last_24h"`
	ComplaintsLast24h          int   `json:"complaints_last_24h"`
	WebhookFailuresLast24h     int   `json:"webhook_failures_last_24h"`
	DedicatedTenants           int   `db:"dedicated_tenants" json:"dedicated_tenants"`
	OpenIncidents              int   `db:"open_incidents" json:"open_incidents"`
	DomainsVerified            int   `json:"domains_verified"`
	DomainsRevoked             int   `json:"domains_revoked"`
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
	0 AS domains_pending_verification,
    (SELECT count(*) FROM mv_audit_events WHERE occurred_at > now() - interval '24 hours') AS audit_events_last_24h,
	0 AS import_jobs_failed_last_24h,
    (SELECT count(*) FROM mv_impersonation_grants WHERE revoked_at IS NULL AND expires_at > now()) AS active_impersonation_grants,
    0 AS mrr_cents, 0 AS arr_cents,
    (SELECT count(*) FROM mv_tenant_infrastructure WHERE mode='dedicated') AS dedicated_tenants,
    (SELECT count(*) FROM mv_platform_incidents WHERE status<>'resolved') AS open_incidents
`)
	if err != nil {
		return PlatformDashboard{}, err
	}
	var tenantIDs []uuid.UUID
	if err := s.db.SelectContext(ctx, &tenantIDs, `SELECT id FROM mv_tenants ORDER BY id`); err != nil {
		return PlatformDashboard{}, err
	}
	for _, tenantID := range tenantIDs {
		tx, err := s.db.BeginTxx(ctx, nil)
		if err != nil {
			return PlatformDashboard{}, err
		}
		if err := setTenantScope(ctx, tx, tenantID); err != nil {
			_ = tx.Rollback()
			return PlatformDashboard{}, err
		}
		var counts struct {
			Domains         int   `db:"domains"`
			Imports         int   `db:"imports"`
			Emails          int64 `db:"emails"`
			Queued          int   `db:"queued"`
			Bounces         int   `db:"bounces"`
			Complaints      int   `db:"complaints"`
			WebhookFailures int   `db:"webhook_failures"`
			MRR             int64 `db:"mrr"`
			DomainsVerified int   `db:"domains_verified"`
			DomainsRevoked  int   `db:"domains_revoked"`
		}
		err = tx.GetContext(ctx, &counts, `SELECT
			(SELECT count(*) FROM mv_tenant_domains WHERE status='pending') AS domains,
			(SELECT count(*) FROM mv_import_jobs WHERE status='failed' AND updated_at>now()-interval '24 hours') AS imports,
			COALESCE((SELECT sum(emails_sent) FROM mv_tenant_usage WHERE period_start=date_trunc('month',now())::date),0) AS emails,
			(SELECT count(*) FROM mv_transactional_messages WHERE status='pending') AS queued,
			(SELECT count(*) FROM bounces WHERE created_at>now()-interval '24 hours') AS bounces,
			(SELECT count(*) FROM mv_complaints WHERE occurred_at>now()-interval '24 hours') AS complaints,
			(SELECT count(*) FROM mv_webhook_deliveries WHERE status='failed' AND created_at>now()-interval '24 hours') AS webhook_failures,
			COALESCE((SELECT sum(amount_cents) FROM mv_invoices WHERE status='paid' AND paid_at>=now()-interval '30 days'),0) AS mrr,
			(SELECT count(*) FROM mv_tenant_domains WHERE status='verified') AS domains_verified,
			(SELECT count(*) FROM mv_tenant_domains WHERE status='revoked') AS domains_revoked`)
		_ = tx.Rollback()
		if err != nil {
			return PlatformDashboard{}, err
		}
		out.DomainsPendingVerification += counts.Domains
		out.ImportJobsFailedLast24h += counts.Imports
		out.EmailsSentThisMonth += counts.Emails
		out.QueuedMessages += counts.Queued
		out.BouncesLast24h += counts.Bounces
		out.ComplaintsLast24h += counts.Complaints
		out.WebhookFailuresLast24h += counts.WebhookFailures
		out.MRRCents += counts.MRR
		out.DomainsVerified += counts.DomainsVerified
		out.DomainsRevoked += counts.DomainsRevoked
	}
	out.ARRCents = out.MRRCents * 12
	return out, nil
}
