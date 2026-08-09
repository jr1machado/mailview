package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound      = errors.New("mailview resource not found")
	ErrConflict      = errors.New("mailview resource already exists")
	ErrInvalid       = errors.New("invalid mailview input")
	ErrUserNotFound  = errors.New("owner or member user not found")
	ErrInvalidRole   = errors.New("role does not belong to tenant")
	slugPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
	validTenantState = map[string]struct{}{TenantStatusActive: {}, TenantStatusSuspended: {}, TenantStatusPending: {}, TenantStatusOffboarded: {}}
)

type Service struct{ db *sqlx.DB }

func New(db *sqlx.DB) *Service { return &Service{db: db} }

func NormalizeSlug(value string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(value))
	if !slugPattern.MatchString(slug) {
		return "", fmt.Errorf("%w: slug must be 3-63 lowercase letters, numbers, or hyphens", ErrInvalid)
	}
	return slug, nil
}

func (s *Service) CreateTenant(ctx context.Context, in CreateTenantInput, actor Actor) (Tenant, error) {
	var out Tenant
	slug, err := NormalizeSlug(in.Slug)
	if err != nil {
		return out, err
	}
	name := strings.TrimSpace(in.Name)
	if len(name) == 0 || len(name) > 160 || in.OwnerUserID < 1 {
		return out, ErrInvalid
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND type = 'user' AND status = 'enabled')`, in.OwnerUserID); err != nil {
		return out, err
	}
	if !exists {
		return out, ErrUserNotFound
	}

	out = Tenant{ID: uuid.Must(uuid.NewV4()), Slug: slug, Name: name, Status: TenantStatusActive}
	if err := tx.GetContext(ctx, &out, `
INSERT INTO mv_tenants (id, slug, name, status) VALUES ($1, $2, $3, $4)
RETURNING id, slug, name, status, created_at, updated_at`, out.ID, out.Slug, out.Name, out.Status); err != nil {
		if isUniqueViolation(err) {
			return Tenant{}, ErrConflict
		}
		return Tenant{}, err
	}

	roles, err := seedTenantRoles(ctx, tx, out.ID)
	if err != nil {
		return Tenant{}, err
	}
	ownerRoleID := roles["Tenant Owner"]
	membership := Membership{ID: uuid.Must(uuid.NewV4()), TenantID: out.ID, UserID: in.OwnerUserID, Status: MembershipStatusActive}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_memberships (id, tenant_id, user_id, status) VALUES ($1, $2, $3, $4)`, membership.ID, membership.TenantID, membership.UserID, membership.Status); err != nil {
		return Tenant{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_membership_roles (membership_id, role_id) VALUES ($1, $2)`, membership.ID, ownerRoleID); err != nil {
		return Tenant{}, err
	}
	if err := appendAudit(ctx, tx, &out.ID, actor, "tenant.create", "tenant", out.ID.String(), "success", "", map[string]any{"slug": out.Slug, "owner_user_id": in.OwnerUserID}); err != nil {
		return Tenant{}, err
	}
	return out, tx.Commit()
}

// EnsureNoBypassRLS refuses to proceed when the connected database role has
// BYPASSRLS: such a role silently ignores every tenant isolation policy, so
// tenant.Begin's set_config-based scoping would stop being enforced without
// any error surfacing anywhere.
func (s *Service) EnsureNoBypassRLS(ctx context.Context) error {
	var bypass bool
	if err := s.db.GetContext(ctx, &bypass, `SELECT rolbypassrls FROM pg_roles WHERE rolname = current_user`); err != nil {
		return fmt.Errorf("checking BYPASSRLS on current database role: %w", err)
	}
	if bypass {
		return errors.New("database role has BYPASSRLS; MailView tenant isolation requires a role without it")
	}
	return nil
}

func (s *Service) ListTenants(ctx context.Context) ([]Tenant, error) {
	out := []Tenant{}
	err := s.db.SelectContext(ctx, &out, `SELECT id, slug, name, status, created_at, updated_at FROM mv_tenants ORDER BY created_at DESC`)
	return out, err
}

func (s *Service) GetTenant(ctx context.Context, id uuid.UUID) (Tenant, error) {
	var out Tenant
	if err := s.db.GetContext(ctx, &out, `SELECT id, slug, name, status, created_at, updated_at FROM mv_tenants WHERE id = $1`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tenant{}, ErrNotFound
		}
		return Tenant{}, err
	}
	return out, nil
}

func (s *Service) GetTenantBySlug(ctx context.Context, slug string) (Tenant, error) {
	var out Tenant
	err := s.db.GetContext(ctx, &out, `SELECT id, slug, name, status, created_at, updated_at FROM mv_tenants WHERE slug = $1`, strings.ToLower(strings.TrimSpace(slug)))
	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	return out, err
}

func (s *Service) HasActiveMembership(ctx context.Context, tenantID uuid.UUID, userID int) (bool, error) {
	var ok bool
	err := s.db.GetContext(ctx, &ok, `SELECT EXISTS(SELECT 1 FROM mv_memberships WHERE tenant_id = $1 AND user_id = $2 AND status = 'active')`, tenantID, userID)
	return ok, err
}

func (s *Service) UpdateTenantStatus(ctx context.Context, id uuid.UUID, status string, actor Actor) (Tenant, error) {
	status = strings.TrimSpace(status)
	if _, ok := validTenantState[status]; !ok {
		return Tenant{}, ErrInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Tenant{}, err
	}
	defer tx.Rollback()
	var out Tenant
	if err := tx.GetContext(ctx, &out, `UPDATE mv_tenants SET status = $2, updated_at = now() WHERE id = $1 RETURNING id, slug, name, status, created_at, updated_at`, id, status); err != nil {
		if isNoRows(err) {
			return Tenant{}, ErrNotFound
		}
		return Tenant{}, err
	}
	if err := appendAudit(ctx, tx, &id, actor, "tenant.status.update", "tenant", id.String(), "success", "", map[string]any{"status": status}); err != nil {
		return Tenant{}, err
	}
	return out, tx.Commit()
}

func (s *Service) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error) {
	out := []Role{}
	err := s.db.SelectContext(ctx, &out, `SELECT id, tenant_id, scope, name, is_system FROM mv_roles WHERE tenant_id = $1 ORDER BY name`, tenantID)
	return out, err
}

func (s *Service) ListMemberships(ctx context.Context, tenantID uuid.UUID) ([]Membership, error) {
	out := []Membership{}
	err := s.db.SelectContext(ctx, &out, `SELECT id, tenant_id, user_id, status, created_at, updated_at FROM mv_memberships WHERE tenant_id = $1 ORDER BY created_at ASC`, tenantID)
	return out, err
}

func (s *Service) CreateMembership(ctx context.Context, tenantID uuid.UUID, in CreateMembershipInput, actor Actor) (Membership, error) {
	if in.UserID < 1 || len(in.RoleIDs) == 0 {
		return Membership{}, ErrInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Membership{}, err
	}
	defer tx.Rollback()
	if err := validateTenantAndUser(ctx, tx, tenantID, in.UserID); err != nil {
		return Membership{}, err
	}
	if err := validateRoles(ctx, tx, tenantID, in.RoleIDs); err != nil {
		return Membership{}, err
	}
	out := Membership{ID: uuid.Must(uuid.NewV4()), TenantID: tenantID, UserID: in.UserID, Status: MembershipStatusActive}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_memberships (id, tenant_id, user_id, status) VALUES ($1, $2, $3, $4)`, out.ID, out.TenantID, out.UserID, out.Status); err != nil {
		if isUniqueViolation(err) {
			return Membership{}, ErrConflict
		}
		return Membership{}, err
	}
	for _, roleID := range in.RoleIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mv_membership_roles (membership_id, role_id) VALUES ($1, $2)`, out.ID, roleID); err != nil {
			return Membership{}, err
		}
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "membership.create", "membership", out.ID.String(), "success", "", map[string]any{"user_id": out.UserID}); err != nil {
		return Membership{}, err
	}
	return out, tx.Commit()
}

// ReplaceMembershipRoles returns the affected user's ID so the caller can
// invalidate their sessions (Fase-4.md 10.4: role changes must invalidate
// sensitive sessions).
func (s *Service) ReplaceMembershipRoles(ctx context.Context, tenantID, membershipID uuid.UUID, roleIDs []uuid.UUID, actor Actor) (int, error) {
	if len(roleIDs) == 0 {
		return 0, ErrInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var userID int
	if err := tx.GetContext(ctx, &userID, `SELECT user_id FROM mv_memberships WHERE id = $1 AND tenant_id = $2`, membershipID, tenantID); err != nil {
		if isNoRows(err) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if err := validateRoles(ctx, tx, tenantID, roleIDs); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mv_membership_roles WHERE membership_id = $1`, membershipID); err != nil {
		return 0, err
	}
	for _, roleID := range roleIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mv_membership_roles (membership_id, role_id) VALUES ($1, $2)`, membershipID, roleID); err != nil {
			return 0, err
		}
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "membership.roles.replace", "membership", membershipID.String(), "success", "", nil); err != nil {
		return 0, err
	}
	return userID, tx.Commit()
}

func (s *Service) ListAuditEvents(ctx context.Context, tenantID uuid.UUID, limit int) ([]AuditEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	out := []AuditEvent{}
	err := s.db.SelectContext(ctx, &out, `SELECT id, occurred_at, tenant_id, actor_user_id, action, resource_type, resource_id, request_id, source_ip::text AS source_ip, user_agent, result, reason, metadata FROM mv_audit_events WHERE tenant_id = $1 ORDER BY occurred_at DESC LIMIT $2`, tenantID, limit)
	return out, err
}

// GenerateRecoveryCodes replaces unused recovery codes. The cleartext values
// are returned once and only bcrypt hashes are retained.
func (s *Service) GenerateRecoveryCodes(ctx context.Context, userID int, actor Actor) ([]string, error) {
	if userID < 1 {
		return nil, ErrInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mv_mfa_recovery_codes WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return nil, err
	}
	codes := make([]string, 10)
	for i := range codes {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mv_mfa_recovery_codes (id, user_id, code_hash) VALUES ($1, $2, $3)`, uuid.Must(uuid.NewV4()), userID, string(hash)); err != nil {
			return nil, err
		}
		codes[i] = code
	}
	if err := appendAudit(ctx, tx, nil, actor, "mfa.recovery_codes.generate", "user", fmt.Sprintf("%d", userID), "success", "", nil); err != nil {
		return nil, err
	}
	return codes, tx.Commit()
}

// UseRecoveryCode consumes one recovery code exactly once. Codes are compared
// with bcrypt inside a transaction so concurrent login attempts cannot reuse
// the same code.
func (s *Service) UseRecoveryCode(ctx context.Context, userID int, code string, actor Actor) (bool, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if userID < 1 || len(code) < 8 || len(code) > 32 {
		return false, nil
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var candidates []struct {
		ID   uuid.UUID `db:"id"`
		Hash string    `db:"code_hash"`
	}
	if err := tx.SelectContext(ctx, &candidates, `SELECT id, code_hash FROM mv_mfa_recovery_codes WHERE user_id = $1 AND used_at IS NULL FOR UPDATE`, userID); err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		if bcrypt.CompareHashAndPassword([]byte(candidate.Hash), []byte(code)) != nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE mv_mfa_recovery_codes SET used_at = now() WHERE id = $1 AND used_at IS NULL`, candidate.ID); err != nil {
			return false, err
		}
		if err := appendAudit(ctx, tx, nil, actor, "mfa.recovery_code.use", "user", fmt.Sprintf("%d", userID), "success", "", nil); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	return false, tx.Commit()
}

func seedTenantRoles(ctx context.Context, tx *sqlx.Tx, tenantID uuid.UUID) (map[string]uuid.UUID, error) {
	permissions := map[string]string{
		"campaign.create.tenant": "Create campaigns", "campaign.approve.tenant": "Approve campaigns", "campaign.read.tenant": "Read campaigns", "campaign.manage.tenant": "Manage campaigns", "campaign.send.tenant": "Send campaigns", "analytics.read.tenant": "Read analytics",
		"subscriber.read.tenant": "Read subscribers", "subscriber.manage.tenant": "Manage subscribers", "subscriber.import.tenant": "Import subscribers", "subscriber.export.tenant": "Export subscribers",
		"list.read.tenant": "Read lists", "list.manage.tenant": "Manage lists", "template.read.tenant": "Read templates", "template.manage.tenant": "Manage templates", "media.read.tenant": "Read media", "media.manage.tenant": "Manage media", "bounce.read.tenant": "Read bounces", "bounce.manage.tenant": "Manage bounces",
		"domain.manage.tenant": "Manage domains", "user.invite.tenant": "Invite users", "user.manage.tenant": "Manage users",
		"audit.read.tenant": "Read audit events", "smtp.manage.tenant": "Manage SMTP", "billing.read.tenant": "Read billing", "billing.manage.tenant": "Manage billing",
	}
	for code, description := range permissions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mv_permissions (code, description) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`, code, description); err != nil {
			return nil, err
		}
	}
	roles := map[string][]string{
		"Tenant Owner":     allPermissionCodes(permissions),
		"Tenant Admin":     {"campaign.create.tenant", "campaign.approve.tenant", "campaign.read.tenant", "campaign.manage.tenant", "campaign.send.tenant", "analytics.read.tenant", "subscriber.read.tenant", "subscriber.manage.tenant", "subscriber.import.tenant", "subscriber.export.tenant", "list.read.tenant", "list.manage.tenant", "template.read.tenant", "template.manage.tenant", "media.read.tenant", "media.manage.tenant", "bounce.read.tenant", "bounce.manage.tenant", "domain.manage.tenant", "user.invite.tenant", "user.manage.tenant", "audit.read.tenant", "smtp.manage.tenant"},
		"Campaign Manager": {"campaign.create.tenant", "campaign.approve.tenant", "campaign.read.tenant", "campaign.manage.tenant", "campaign.send.tenant", "analytics.read.tenant", "template.read.tenant", "template.manage.tenant", "media.read.tenant", "media.manage.tenant", "bounce.read.tenant"},
		"Operator":         {"campaign.create.tenant", "campaign.read.tenant", "campaign.manage.tenant", "subscriber.read.tenant", "subscriber.manage.tenant", "subscriber.import.tenant", "list.read.tenant", "list.manage.tenant", "template.read.tenant", "template.manage.tenant", "media.read.tenant", "media.manage.tenant"},
		"Analyst":          {"campaign.read.tenant", "analytics.read.tenant", "subscriber.read.tenant", "subscriber.export.tenant", "list.read.tenant", "template.read.tenant", "media.read.tenant", "bounce.read.tenant", "audit.read.tenant"},
		"Viewer":           {"campaign.read.tenant", "analytics.read.tenant", "subscriber.read.tenant", "list.read.tenant", "template.read.tenant", "media.read.tenant", "bounce.read.tenant"},
		"Billing Manager":  {"billing.read.tenant", "billing.manage.tenant"},
	}
	out := make(map[string]uuid.UUID, len(roles))
	for name, rolePermissions := range roles {
		id := uuid.Must(uuid.NewV4())
		if _, err := tx.ExecContext(ctx, `INSERT INTO mv_roles (id, tenant_id, scope, name, is_system) VALUES ($1, $2, 'tenant', $3, true)`, id, tenantID, name); err != nil {
			return nil, err
		}
		for _, code := range rolePermissions {
			if _, err := tx.ExecContext(ctx, `INSERT INTO mv_role_permissions (role_id, permission_code) VALUES ($1, $2)`, id, code); err != nil {
				return nil, err
			}
		}
		out[name] = id
	}
	return out, nil
}

func allPermissionCodes(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for code := range values {
		out = append(out, code)
	}
	return out
}

func validateTenantAndUser(ctx context.Context, tx *sqlx.Tx, tenantID uuid.UUID, userID int) error {
	var tenant, user bool
	if err := tx.GetContext(ctx, &tenant, `SELECT EXISTS(SELECT 1 FROM mv_tenants WHERE id = $1)`, tenantID); err != nil {
		return err
	}
	if !tenant {
		return ErrNotFound
	}
	if err := tx.GetContext(ctx, &user, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND type = 'user' AND status = 'enabled')`, userID); err != nil {
		return err
	}
	if !user {
		return ErrUserNotFound
	}
	return nil
}

func validateRoles(ctx context.Context, tx *sqlx.Tx, tenantID uuid.UUID, roleIDs []uuid.UUID) error {
	var count int
	if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM mv_roles WHERE tenant_id = $1 AND id = ANY($2)`, tenantID, pq.Array(roleIDs)); err != nil {
		return err
	}
	if count != len(roleIDs) {
		return ErrInvalidRole
	}
	return nil
}

func appendAudit(ctx context.Context, tx *sqlx.Tx, tenantID *uuid.UUID, actor Actor, action, resourceType, resourceID, result, reason string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	var ip any
	if parsed := net.ParseIP(actor.SourceIP); parsed != nil {
		ip = parsed.String()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mv_audit_events (id, tenant_id, actor_user_id, action, resource_type, resource_id, request_id, source_ip, user_agent, result, reason, metadata) VALUES ($1, $2, NULLIF($3, 0), $4, $5, $6, $7, $8::inet, $9, $10, $11, $12::jsonb)`, uuid.Must(uuid.NewV4()), tenantID, actor.UserID, action, resourceType, resourceID, actor.RequestID, ip, actor.UserAgent, result, reason, string(metadataJSON))
	return err
}

func newRecoveryCode() (string, error) {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	token := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:10])
	return token[:5] + "-" + token[5:10] + "-" + token[10:15], nil
}
func isUniqueViolation(err error) bool { p, ok := err.(*pq.Error); return ok && p.Code == "23505" }
func isNoRows(err error) bool          { return errors.Is(err, sql.ErrNoRows) }
