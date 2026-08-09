package control

import (
	"context"
	"errors"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
)

var ErrPlanDoesNotAllowCustomRoles = errors.New("tenant plan does not allow custom roles")

// HasTenantPermission reports whether userID's tenant roles in tenantID grant
// permissionCode, honoring Fase-4.md 10.4: an explicit denial on any of the
// user's roles always wins over a grant, no matter which role carries which.
func (s *Service) HasTenantPermission(ctx context.Context, tenantID uuid.UUID, userID int, permissionCode string) (bool, error) {
	var denied bool
	if err := s.db.GetContext(ctx, &denied, `
SELECT EXISTS(
    SELECT 1
    FROM mv_memberships m
    JOIN mv_membership_roles mr ON mr.membership_id = m.id
    JOIN mv_role_permission_denials d ON d.role_id = mr.role_id AND d.permission_code = $3
    WHERE m.tenant_id = $1 AND m.user_id = $2 AND m.status = 'active'
)`, tenantID, userID, permissionCode); err != nil {
		return false, err
	}
	if denied {
		return false, nil
	}

	var granted bool
	err := s.db.GetContext(ctx, &granted, `
SELECT EXISTS(
    SELECT 1
    FROM mv_memberships m
    JOIN mv_membership_roles mr ON mr.membership_id = m.id
    JOIN mv_role_permissions rp ON rp.role_id = mr.role_id AND rp.permission_code = $3
    WHERE m.tenant_id = $1 AND m.user_id = $2 AND m.status = 'active'
)`, tenantID, userID, permissionCode)
	return granted, err
}

func (s *Service) ListTenantPermissions(ctx context.Context, tenantID uuid.UUID, userID int) ([]string, error) {
	out := []string{}
	err := s.db.SelectContext(ctx, &out, `
SELECT DISTINCT rp.permission_code
FROM mv_memberships m
JOIN mv_membership_roles mr ON mr.membership_id=m.id
JOIN mv_role_permissions rp ON rp.role_id=mr.role_id
WHERE m.tenant_id=$1 AND m.user_id=$2 AND m.status='active'
AND NOT EXISTS (
 SELECT 1 FROM mv_membership_roles mr2
 JOIN mv_role_permission_denials d ON d.role_id=mr2.role_id
 WHERE mr2.membership_id=m.id AND d.permission_code=rp.permission_code
)
ORDER BY rp.permission_code`, tenantID, userID)
	return out, err
}

// DenyRolePermission adds an explicit denial that overrides any grant of the
// same permission on roleID, for every role a membership might combine.
func (s *Service) DenyRolePermission(ctx context.Context, tenantID, roleID uuid.UUID, permissionCode string, actor Actor) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var belongs bool
	if err := tx.GetContext(ctx, &belongs, `SELECT EXISTS(SELECT 1 FROM mv_roles WHERE id = $1 AND tenant_id = $2)`, roleID, tenantID); err != nil {
		return err
	}
	if !belongs {
		return ErrInvalidRole
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_role_permission_denials (role_id, permission_code) VALUES ($1, $2) ON CONFLICT DO NOTHING`, roleID, permissionCode); err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "role_permission.deny", "role", roleID.String(), "success", "", map[string]any{"permission_code": permissionCode}); err != nil {
		return err
	}
	return tx.Commit()
}

// AllowRolePermission removes a previously added explicit denial. It does
// not itself grant the permission; the role must still have a matching row
// in mv_role_permissions for HasTenantPermission to return true.
func (s *Service) AllowRolePermission(ctx context.Context, tenantID, roleID uuid.UUID, permissionCode string, actor Actor) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
DELETE FROM mv_role_permission_denials
WHERE role_id = $1 AND permission_code = $2
    AND role_id IN (SELECT id FROM mv_roles WHERE tenant_id = $3)`, roleID, permissionCode, tenantID); err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "role_permission.allow", "role", roleID.String(), "success", "", map[string]any{"permission_code": permissionCode}); err != nil {
		return err
	}
	return tx.Commit()
}

var customRolePlans = map[string]struct{}{"growth": {}, "enterprise": {}}

type CreateTenantRoleInput struct {
	Name            string   `json:"name"`
	PermissionCodes []string `json:"permission_codes"`
}

// CreateTenantRole adds a custom role beyond the seven system defaults.
// Fase-4.md 10.4 restricts this to tenants on a plan that allows it; a
// tenant without a quota row yet (never assigned a plan) defaults to
// "starter" via GetTenantQuota and is rejected same as an explicit starter.
func (s *Service) CreateTenantRole(ctx context.Context, tenantID uuid.UUID, in CreateTenantRoleInput, actor Actor) (Role, error) {
	name := strings.TrimSpace(in.Name)
	if len(name) < 1 || len(name) > 100 || len(in.PermissionCodes) == 0 {
		return Role{}, ErrInvalid
	}
	quota, err := s.GetTenantQuota(ctx, tenantID)
	if err != nil {
		return Role{}, err
	}
	if _, ok := customRolePlans[quota.PlanCode]; !ok {
		return Role{}, ErrPlanDoesNotAllowCustomRoles
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Role{}, err
	}
	defer tx.Rollback()

	var permCount int
	if err := tx.GetContext(ctx, &permCount, `SELECT COUNT(*) FROM mv_permissions WHERE code = ANY($1)`, pq.Array(in.PermissionCodes)); err != nil {
		return Role{}, err
	}
	if permCount != len(in.PermissionCodes) {
		return Role{}, ErrInvalid
	}

	role := Role{ID: uuid.Must(uuid.NewV4()), TenantID: &tenantID, Scope: "tenant", Name: name, IsSystem: false}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_roles (id, tenant_id, scope, name, is_system) VALUES ($1, $2, 'tenant', $3, false)`, role.ID, tenantID, name); err != nil {
		if isUniqueViolation(err) {
			return Role{}, ErrConflict
		}
		return Role{}, err
	}
	for _, code := range in.PermissionCodes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mv_role_permissions (role_id, permission_code) VALUES ($1, $2)`, role.ID, code); err != nil {
			return Role{}, err
		}
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "role.create_custom", "role", role.ID.String(), "success", "", map[string]any{"name": name, "plan_code": quota.PlanCode}); err != nil {
		return Role{}, err
	}
	return role, tx.Commit()
}
