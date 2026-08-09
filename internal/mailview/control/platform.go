package control

import (
	"context"
	"fmt"

	"github.com/gofrs/uuid/v5"
)

type PlatformRoleAssignment struct {
	ID        uuid.UUID `db:"id" json:"id"`
	UserID    int       `db:"user_id" json:"user_id"`
	RoleID    uuid.UUID `db:"role_id" json:"role_id"`
	RoleName  string    `db:"role_name" json:"role_name"`
	CreatedAt string    `db:"created_at" json:"created_at"`
}

// ListPlatformRoles returns the fixed set of platform-scope roles (scope =
// 'platform'). Unlike tenant roles, these are seeded by migration and are
// not created per tenant.
func (s *Service) ListPlatformRoles(ctx context.Context) ([]Role, error) {
	out := []Role{}
	err := s.db.SelectContext(ctx, &out, `SELECT id, tenant_id, scope, name, is_system FROM mv_roles WHERE scope = 'platform' ORDER BY name`)
	return out, err
}

// ListPlatformAssignments returns every user with at least one platform role.
func (s *Service) ListPlatformAssignments(ctx context.Context) ([]PlatformRoleAssignment, error) {
	out := []PlatformRoleAssignment{}
	err := s.db.SelectContext(ctx, &out, `
SELECT a.id, a.user_id, a.role_id, r.name AS role_name, a.created_at::text AS created_at
FROM mv_platform_role_assignments a
JOIN mv_roles r ON r.id = a.role_id
ORDER BY a.created_at ASC`)
	return out, err
}

// AssignPlatformRole grants a platform-scope role to a user. Only roles with
// scope = 'platform' are accepted; tenant roles cannot be assigned here.
func (s *Service) AssignPlatformRole(ctx context.Context, userID int, roleID uuid.UUID, actor Actor) (PlatformRoleAssignment, error) {
	if userID < 1 {
		return PlatformRoleAssignment{}, ErrInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return PlatformRoleAssignment{}, err
	}
	defer tx.Rollback()

	var role bool
	if err := tx.GetContext(ctx, &role, `SELECT EXISTS(SELECT 1 FROM mv_roles WHERE id = $1 AND scope = 'platform')`, roleID); err != nil {
		return PlatformRoleAssignment{}, err
	}
	if !role {
		return PlatformRoleAssignment{}, ErrInvalidRole
	}
	var user bool
	if err := tx.GetContext(ctx, &user, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND type = 'user' AND status = 'enabled')`, userID); err != nil {
		return PlatformRoleAssignment{}, err
	}
	if !user {
		return PlatformRoleAssignment{}, ErrUserNotFound
	}

	out := PlatformRoleAssignment{ID: uuid.Must(uuid.NewV4()), UserID: userID, RoleID: roleID}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_platform_role_assignments (id, user_id, role_id) VALUES ($1, $2, $3) ON CONFLICT (user_id, role_id) DO NOTHING`, out.ID, userID, roleID); err != nil {
		return PlatformRoleAssignment{}, err
	}
	if err := appendAudit(ctx, tx, nil, actor, "platform_role.assign", "user", fmt.Sprintf("%d", userID), "success", "", map[string]any{"role_id": roleID.String()}); err != nil {
		return PlatformRoleAssignment{}, err
	}
	return out, tx.Commit()
}

// RevokePlatformRole removes a platform-scope role from a user.
func (s *Service) RevokePlatformRole(ctx context.Context, userID int, roleID uuid.UUID, actor Actor) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM mv_platform_role_assignments WHERE user_id = $1 AND role_id = $2`, userID, roleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := appendAudit(ctx, tx, nil, actor, "platform_role.revoke", "user", fmt.Sprintf("%d", userID), "success", "", map[string]any{"role_id": roleID.String()}); err != nil {
		return err
	}
	return tx.Commit()
}

// HasPlatformPermission reports whether userID holds a platform role granting
// permissionCode. It is the non-legacy path for gating MailView platform
// administration, meant to run alongside the existing Super Admin bridge.
func (s *Service) HasPlatformPermission(ctx context.Context, userID int, permissionCode string) (bool, error) {
	var ok bool
	err := s.db.GetContext(ctx, &ok, `
SELECT EXISTS(
    SELECT 1
    FROM mv_platform_role_assignments a
    JOIN mv_role_permissions rp ON rp.role_id = a.role_id
    WHERE a.user_id = $1 AND rp.permission_code = $2
)`, userID, permissionCode)
	return ok, err
}

// ListPlatformPermissions returns the effective platform capabilities used
// by the admin UI. Platform permissions are additive because platform roles
// are fixed, migration-seeded roles and cannot carry tenant-defined denials.
func (s *Service) ListPlatformPermissions(ctx context.Context, userID int) ([]string, error) {
	out := []string{}
	err := s.db.SelectContext(ctx, &out, `
SELECT DISTINCT rp.permission_code
FROM mv_platform_role_assignments a
JOIN mv_role_permissions rp ON rp.role_id = a.role_id
WHERE a.user_id = $1
ORDER BY rp.permission_code`, userID)
	return out, err
}
