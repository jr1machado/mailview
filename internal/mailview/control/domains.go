package control

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// setTenantScope binds app.tenant_id for the lifetime of tx, the same
// set_config mechanism tenant.Begin uses. mv_tenant_domains has FORCE ROW
// LEVEL SECURITY, but control-plane admin operations run outside the
// request-scoped tenant.Context that dataplane/importjob rely on: each call
// here already receives its target tenantID as an explicit parameter, so the
// scope is set directly on the transaction instead.
func setTenantScope(ctx context.Context, tx *sqlx.Tx, tenantID uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantID.String())
	return err
}

var validDomainPurpose = map[string]struct{}{
	"portal": {}, "tracking": {}, "sending": {}, "return_path": {}, "public_forms": {},
}

var hostnamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

const tenantDomainColumns = `id, tenant_id, hostname, purpose, verification_method, verification_token,
verification_name, verification_value, status, tls_status, last_verified_at, last_checked_at,
next_check_at, failure_reason, certificate_ref, certificate_revoked_at, owner_user_id, created_by, created_at, updated_at`

type TenantDomain struct {
	ID                   uuid.UUID  `db:"id" json:"id"`
	TenantID             uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	Hostname             string     `db:"hostname" json:"hostname"`
	Purpose              string     `db:"purpose" json:"purpose"`
	VerificationMethod   string     `db:"verification_method" json:"verification_method"`
	VerificationToken    string     `db:"verification_token" json:"verification_token"`
	VerificationName     string     `db:"verification_name" json:"verification_name"`
	VerificationValue    string     `db:"verification_value" json:"verification_value"`
	Status               string     `db:"status" json:"status"`
	TLSStatus            string     `db:"tls_status" json:"tls_status"`
	LastVerifiedAt       *time.Time `db:"last_verified_at" json:"last_verified_at,omitempty"`
	LastCheckedAt        *time.Time `db:"last_checked_at" json:"last_checked_at,omitempty"`
	NextCheckAt          *time.Time `db:"next_check_at" json:"next_check_at,omitempty"`
	FailureReason        string     `db:"failure_reason" json:"failure_reason,omitempty"`
	CertificateRef       string     `db:"certificate_ref" json:"certificate_ref,omitempty"`
	CertificateRevokedAt *time.Time `db:"certificate_revoked_at" json:"certificate_revoked_at,omitempty"`
	OwnerUserID          int        `db:"owner_user_id" json:"owner_user_id"`
	CreatedBy            int        `db:"created_by" json:"created_by"`
	CreatedAt            time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at" json:"updated_at"`
}

type CreateTenantDomainInput struct {
	Hostname           string `json:"hostname"`
	Purpose            string `json:"purpose"`
	VerificationMethod string `json:"verification_method"`
}

// CreateTenantDomain registers a hostname pending DNS verification and returns
// the server-generated TXT/CNAME challenge. Creation never activates routing.
func (s *Service) CreateTenantDomain(ctx context.Context, tenantID uuid.UUID, in CreateTenantDomainInput, actor Actor) (TenantDomain, error) {
	hostname := strings.ToLower(strings.TrimSpace(in.Hostname))
	if !validTenantHostname(hostname) {
		return TenantDomain{}, ErrInvalid
	}
	if _, ok := validDomainPurpose[in.Purpose]; !ok {
		return TenantDomain{}, ErrInvalid
	}
	method := in.VerificationMethod
	if method == "" {
		method = "txt"
	}
	if method != "txt" && method != "cname" {
		return TenantDomain{}, ErrInvalid
	}

	token, err := newDomainVerificationToken()
	if err != nil {
		return TenantDomain{}, err
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return TenantDomain{}, err
	}
	defer tx.Rollback()
	if err := setTenantScope(ctx, tx, tenantID); err != nil {
		return TenantDomain{}, err
	}

	var tenantExists bool
	if err := tx.GetContext(ctx, &tenantExists, `SELECT EXISTS(SELECT 1 FROM mv_tenants WHERE id = $1)`, tenantID); err != nil {
		return TenantDomain{}, err
	}
	if !tenantExists {
		return TenantDomain{}, ErrNotFound
	}
	var ownerUserID int
	if err := tx.GetContext(ctx, &ownerUserID, `SELECT m.user_id FROM mv_memberships m
		JOIN mv_membership_roles mr ON mr.membership_id=m.id JOIN mv_roles r ON r.id=mr.role_id
		WHERE m.tenant_id=$1 AND m.status='active' AND r.name='Tenant Owner' ORDER BY m.created_at LIMIT 1`, tenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TenantDomain{}, ErrInvalid
		}
		return TenantDomain{}, err
	}

	out := TenantDomain{
		ID: uuid.Must(uuid.NewV4()), TenantID: tenantID, Hostname: hostname, Purpose: in.Purpose,
		VerificationMethod: method, VerificationToken: token, Status: "pending", TLSStatus: "none", CreatedBy: actor.UserID, OwnerUserID: ownerUserID,
	}
	out.VerificationName, out.VerificationValue = domainVerificationRecord(out)
	if err := tx.GetContext(ctx, &out, `
INSERT INTO mv_tenant_domains (id, tenant_id, hostname, purpose, verification_method, verification_token, verification_name, verification_value, created_by, owner_user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING `+tenantDomainColumns,
		out.ID, out.TenantID, out.Hostname, out.Purpose, out.VerificationMethod, out.VerificationToken, out.VerificationName, out.VerificationValue, out.CreatedBy, out.OwnerUserID); err != nil {
		if isUniqueViolation(err) {
			return TenantDomain{}, ErrConflict
		}
		return TenantDomain{}, err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "tenant_domain.create", "tenant_domain", out.ID.String(), "success", "", map[string]any{"hostname": hostname, "purpose": in.Purpose}); err != nil {
		return TenantDomain{}, err
	}
	return out, tx.Commit()
}

func (s *Service) ListTenantDomains(ctx context.Context, tenantID uuid.UUID) ([]TenantDomain, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := setTenantScope(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	out := []TenantDomain{}
	if err := tx.SelectContext(ctx, &out, `
SELECT `+tenantDomainColumns+`
FROM mv_tenant_domains WHERE tenant_id = $1 ORDER BY created_at ASC`, tenantID); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

// GetTenantByVerifiedHostname resolves a custom hostname only after ownership
// verification. Revoked/pending domains never participate in routing.
func (s *Service) GetTenantByVerifiedHostname(ctx context.Context, hostname string) (Tenant, error) {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return Tenant{}, ErrNotFound
	}

	// mv_tenant_domains is FORCE RLS, so hostname routing must not issue an
	// unscoped query (which correctly sees zero rows with the application
	// role). Iterate active tenant IDs from the Control Plane and probe the
	// indexed hostname inside an explicitly scoped transaction.
	var tenantIDs []uuid.UUID
	if err := s.db.SelectContext(ctx, &tenantIDs, `SELECT id FROM mv_tenants WHERE status='active' ORDER BY id`); err != nil {
		return Tenant{}, err
	}
	for _, tenantID := range tenantIDs {
		tx, err := s.db.BeginTxx(ctx, nil)
		if err != nil {
			return Tenant{}, err
		}
		if err := setTenantScope(ctx, tx, tenantID); err != nil {
			_ = tx.Rollback()
			return Tenant{}, err
		}
		var out Tenant
		err = tx.GetContext(ctx, &out, `
SELECT t.id,t.slug,t.name,t.status,t.created_at,t.updated_at
FROM mv_tenant_domains d JOIN mv_tenants t ON t.id=d.tenant_id
WHERE d.hostname=$1 AND d.status='verified' AND t.status='active'`, hostname)
		_ = tx.Rollback()
		if err == nil {
			return out, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Tenant{}, err
		}
	}
	return Tenant{}, ErrNotFound
}

// MarkTenantDomainVerified verifies the live DNS record before activation.
// The historical name is retained for API compatibility.
func (s *Service) MarkTenantDomainVerified(ctx context.Context, tenantID, domainID uuid.UUID, actor Actor) (TenantDomain, error) {
	return s.VerifyTenantDomainDNS(ctx, tenantID, domainID, actor)
}

func (s *Service) RevokeTenantDomain(ctx context.Context, tenantID, domainID uuid.UUID, actor Actor) (TenantDomain, error) {
	return s.updateTenantDomainStatus(ctx, tenantID, domainID, "revoked", actor, "tenant_domain.revoke", "ownership revoked")
}

// SetTenantDomainTLSStatus is called by the certificate controller. An issued
// certificate is accepted only for a DNS-verified hostname; revocation clears
// the certificate reference and timestamps the action.
func (s *Service) SetTenantDomainTLSStatus(ctx context.Context, tenantID, domainID uuid.UUID, status, certificateRef string, actor Actor) (TenantDomain, error) {
	status = strings.TrimSpace(status)
	certificateRef = strings.TrimSpace(certificateRef)
	if status != "pending" && status != "issued" && status != "failed" && status != "revoked" {
		return TenantDomain{}, ErrInvalid
	}
	if status == "issued" && certificateRef == "" {
		return TenantDomain{}, ErrInvalid
	}
	if status == "issued" {
		domain, err := s.getTenantDomain(ctx, tenantID, domainID)
		if err != nil {
			return TenantDomain{}, err
		}
		if domain.Status != "verified" {
			return TenantDomain{}, ErrInvalid
		}
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return TenantDomain{}, err
	}
	defer tx.Rollback()
	if err := setTenantScope(ctx, tx, tenantID); err != nil {
		return TenantDomain{}, err
	}
	var out TenantDomain
	err = tx.GetContext(ctx, &out, `UPDATE mv_tenant_domains SET tls_status=$3,
		certificate_ref=CASE WHEN $3='issued' THEN $4 WHEN $3='revoked' THEN '' ELSE certificate_ref END,
		certificate_revoked_at=CASE WHEN $3='revoked' THEN now() WHEN $3='issued' THEN NULL ELSE certificate_revoked_at END,
		updated_at=now()
		WHERE tenant_id=$1 AND id=$2 RETURNING `+tenantDomainColumns,
		tenantID, domainID, status, certificateRef)
	if errors.Is(err, sql.ErrNoRows) {
		return TenantDomain{}, ErrNotFound
	}
	if err != nil {
		return TenantDomain{}, err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "tenant_domain.tls_status", "tenant_domain", domainID.String(), "success", "", map[string]any{"tls_status": status}); err != nil {
		return TenantDomain{}, err
	}
	return out, tx.Commit()
}

func (s *Service) updateTenantDomainStatus(ctx context.Context, tenantID, domainID uuid.UUID, status string, actor Actor, action, reason string) (TenantDomain, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return TenantDomain{}, err
	}
	defer tx.Rollback()
	if err := setTenantScope(ctx, tx, tenantID); err != nil {
		return TenantDomain{}, err
	}

	var out TenantDomain
	verifiedAt, nextCheck, tlsStatus, revokedAt, certificateRef := "last_verified_at", "NULL", "tls_status", "certificate_revoked_at", "certificate_ref"
	if status == "verified" {
		verifiedAt = "now()"
		nextCheck = "now() + interval '24 hours'"
		tlsStatus = "CASE WHEN tls_status='issued' THEN 'issued' ELSE 'pending' END"
		revokedAt = "NULL"
	} else if status == "revoked" {
		tlsStatus = "'revoked'"
		revokedAt = "now()"
		certificateRef = "''"
	} else if status == "failed" {
		tlsStatus = "'revoked'"
		revokedAt = "now()"
		certificateRef = "''"
	}
	q := `UPDATE mv_tenant_domains SET status=$3, last_verified_at=` + verifiedAt + `, last_checked_at=now(),
next_check_at=` + nextCheck + `, failure_reason=$4, tls_status=` + tlsStatus + `, certificate_ref=` + certificateRef + `,
certificate_revoked_at=` + revokedAt + `, updated_at=now()
WHERE id = $1 AND tenant_id = $2
RETURNING ` + tenantDomainColumns
	if err := tx.GetContext(ctx, &out, q, domainID, tenantID, status, reason); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TenantDomain{}, ErrNotFound
		}
		return TenantDomain{}, err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, action, "tenant_domain", domainID.String(), "success", "", nil); err != nil {
		return TenantDomain{}, err
	}
	return out, tx.Commit()
}

func (s *Service) VerifyTenantDomainDNS(ctx context.Context, tenantID, domainID uuid.UUID, actor Actor) (TenantDomain, error) {
	domain, err := s.getTenantDomain(ctx, tenantID, domainID)
	if err != nil {
		return TenantDomain{}, err
	}
	verified, reason := s.lookupDomainOwnership(ctx, domain)
	if !verified {
		return s.updateTenantDomainStatus(ctx, tenantID, domainID, "failed", actor, "tenant_domain.verify", reason)
	}
	return s.updateTenantDomainStatus(ctx, tenantID, domainID, "verified", actor, "tenant_domain.verify", "")
}

// RevalidateDueDomains rechecks verified domains whose next_check_at elapsed.
// A failed lookup immediately removes the hostname from routing and marks its
// TLS certificate revoked, preventing domain takeover after DNS ownership moves.
func (s *Service) RevalidateDueDomains(ctx context.Context, actor Actor, limit int) (int, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var tenantIDs []uuid.UUID
	if err := s.db.SelectContext(ctx, &tenantIDs, `SELECT id FROM mv_tenants WHERE status='active' ORDER BY id`); err != nil {
		return 0, err
	}
	checked := 0
	for _, tenantID := range tenantIDs {
		if checked >= limit {
			break
		}
		tx, err := s.db.BeginTxx(ctx, nil)
		if err != nil {
			return checked, err
		}
		if err := setTenantScope(ctx, tx, tenantID); err != nil {
			_ = tx.Rollback()
			return checked, err
		}
		var ids []uuid.UUID
		err = tx.SelectContext(ctx, &ids, `SELECT id FROM mv_tenant_domains WHERE tenant_id=$1 AND status='verified' AND next_check_at<=now() ORDER BY next_check_at LIMIT $2`, tenantID, limit-checked)
		_ = tx.Rollback()
		if err != nil {
			return checked, err
		}
		for _, id := range ids {
			if _, err := s.VerifyTenantDomainDNS(ctx, tenantID, id, actor); err != nil {
				return checked, err
			}
			checked++
		}
	}
	return checked, nil
}

func (s *Service) getTenantDomain(ctx context.Context, tenantID, domainID uuid.UUID) (TenantDomain, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return TenantDomain{}, err
	}
	defer tx.Rollback()
	if err := setTenantScope(ctx, tx, tenantID); err != nil {
		return TenantDomain{}, err
	}
	var out TenantDomain
	if err := tx.GetContext(ctx, &out, `SELECT `+tenantDomainColumns+` FROM mv_tenant_domains WHERE tenant_id=$1 AND id=$2`, tenantID, domainID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TenantDomain{}, ErrNotFound
		}
		return TenantDomain{}, err
	}
	return out, tx.Commit()
}

func (s *Service) lookupDomainOwnership(ctx context.Context, domain TenantDomain) (bool, string) {
	if domain.VerificationMethod == "txt" {
		values, err := s.resolver.LookupTXT(ctx, domain.VerificationName)
		if err != nil {
			return false, fmt.Sprintf("TXT lookup failed: %v", err)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == domain.VerificationValue {
				return true, ""
			}
		}
		return false, "expected TXT value not found"
	}
	target, err := s.resolver.LookupCNAME(ctx, domain.VerificationName)
	if err != nil {
		return false, fmt.Sprintf("CNAME lookup failed: %v", err)
	}
	if strings.TrimSuffix(strings.ToLower(target), ".") != strings.TrimSuffix(strings.ToLower(domain.VerificationValue), ".") {
		return false, "CNAME target does not match"
	}
	return true, ""
}

func domainVerificationRecord(domain TenantDomain) (string, string) {
	if domain.VerificationMethod == "cname" {
		hexToken := strings.TrimPrefix(domain.VerificationToken, "mailview-verify-")
		return domain.Hostname, hexToken + ".verify.mailview.com.br"
	}
	return "_mailview-verification." + domain.Hostname, domain.VerificationToken
}

func validTenantHostname(hostname string) bool {
	if len(hostname) < 4 || len(hostname) > 253 || strings.Contains(hostname, "*") || net.ParseIP(hostname) != nil {
		return false
	}
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") || strings.HasSuffix(hostname, ".local") {
		return false
	}
	return hostnamePattern.MatchString(hostname)
}

func newDomainVerificationToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "mailview-verify-" + hex.EncodeToString(raw), nil
}

type TenantPlan struct {
	Code           string `db:"code" json:"code"`
	Name           string `db:"name" json:"name"`
	MaxSubscribers *int   `db:"max_subscribers" json:"max_subscribers,omitempty"`
	MaxEmailsMonth *int   `db:"max_emails_month" json:"max_emails_month,omitempty"`
	MaxDomains     *int   `db:"max_domains" json:"max_domains,omitempty"`
	MaxSeats       *int   `db:"max_seats" json:"max_seats,omitempty"`
}

func (s *Service) ListTenantPlans(ctx context.Context) ([]TenantPlan, error) {
	out := []TenantPlan{}
	err := s.db.SelectContext(ctx, &out, `SELECT code, name, max_subscribers, max_emails_month, max_domains, max_seats FROM mv_tenant_plans ORDER BY code`)
	return out, err
}

type TenantQuota struct {
	TenantID       uuid.UUID `db:"tenant_id" json:"tenant_id"`
	PlanCode       string    `db:"plan_code" json:"plan_code"`
	MaxSubscribers *int      `db:"max_subscribers" json:"max_subscribers,omitempty"`
	MaxEmailsMonth *int      `db:"max_emails_month" json:"max_emails_month,omitempty"`
	MaxDomains     *int      `db:"max_domains" json:"max_domains,omitempty"`
	MaxSeats       *int      `db:"max_seats" json:"max_seats,omitempty"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// GetTenantQuota returns the tenant's quota row, defaulting it to the
// starter plan on first read so every tenant has one without a separate
// provisioning step.
func (s *Service) GetTenantQuota(ctx context.Context, tenantID uuid.UUID) (TenantQuota, error) {
	var out TenantQuota
	err := s.db.GetContext(ctx, &out, `SELECT tenant_id, plan_code, max_subscribers, max_emails_month, max_domains, max_seats, updated_at FROM mv_tenant_quotas WHERE tenant_id = $1`, tenantID)
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TenantQuota{}, err
	}
	return s.SetTenantQuotaPlan(ctx, tenantID, "starter", Actor{})
}

// SetTenantQuotaPlan assigns tenantID to planCode, resetting any per-tenant
// overrides to that plan's defaults.
func (s *Service) SetTenantQuotaPlan(ctx context.Context, tenantID uuid.UUID, planCode string, actor Actor) (TenantQuota, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return TenantQuota{}, err
	}
	defer tx.Rollback()

	var plan bool
	if err := tx.GetContext(ctx, &plan, `SELECT EXISTS(SELECT 1 FROM mv_tenant_plans WHERE code = $1)`, planCode); err != nil {
		return TenantQuota{}, err
	}
	if !plan {
		return TenantQuota{}, ErrInvalid
	}
	var tenantExists bool
	if err := tx.GetContext(ctx, &tenantExists, `SELECT EXISTS(SELECT 1 FROM mv_tenants WHERE id = $1)`, tenantID); err != nil {
		return TenantQuota{}, err
	}
	if !tenantExists {
		return TenantQuota{}, ErrNotFound
	}

	var out TenantQuota
	if err := tx.GetContext(ctx, &out, `
INSERT INTO mv_tenant_quotas (tenant_id, plan_code, max_subscribers, max_emails_month, max_domains, max_seats)
SELECT $1, code, max_subscribers, max_emails_month, max_domains, max_seats FROM mv_tenant_plans WHERE code = $2
ON CONFLICT (tenant_id) DO UPDATE SET
    plan_code = EXCLUDED.plan_code, max_subscribers = EXCLUDED.max_subscribers, max_emails_month = EXCLUDED.max_emails_month,
    max_domains = EXCLUDED.max_domains, max_seats = EXCLUDED.max_seats, updated_at = now()
RETURNING tenant_id, plan_code, max_subscribers, max_emails_month, max_domains, max_seats, updated_at`, tenantID, planCode); err != nil {
		return TenantQuota{}, err
	}
	if actor.UserID != 0 {
		if err := appendAudit(ctx, tx, &tenantID, actor, "tenant_quota.set_plan", "tenant", tenantID.String(), "success", "", map[string]any{"plan_code": planCode}); err != nil {
			return TenantQuota{}, err
		}
	}
	return out, tx.Commit()
}
