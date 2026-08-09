package control

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/mailview/migrations"
	_ "github.com/lib/pq"
)

// TestFase4Integration is opt-in and covers RBAC explicit-deny precedence,
// custom-role plan gating, and impersonation grant lifecycle (MFA recency,
// TTL bounds, cross-tenant rejection, expiry/revocation).
func TestFase4Integration(t *testing.T) {
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
	// IDs offset from the other test files in this package so they can all
	// run together in one `go test` invocation.
	if _, err := db.ExecContext(ctx, `INSERT INTO roles (id, type, name, permissions) VALUES (3, 'user', 'Super Admin 3', '{}') ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, username, password_login, password, email, name, type, user_role_id, status) VALUES
    (201, 'owner3', true, 'unused', 'owner3@example.test', 'Owner3', 'user', 3, 'enabled'),
    (202, 'member3', true, 'unused', 'member3@example.test', 'Member3', 'user', 3, 'enabled')
ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}

	svc := New(db)
	actor := Actor{UserID: 201, RequestID: "test-request"}

	tenant, err := svc.CreateTenant(ctx, CreateTenantInput{Slug: "acme-rbac", Name: "Acme RBAC", OwnerUserID: 201}, actor)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := svc.ListRoles(ctx, tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ownerRoleID uuid.UUID
	for _, r := range roles {
		if r.Name == "Tenant Owner" {
			ownerRoleID = r.ID
		}
	}
	if ownerRoleID == uuid.Nil {
		t.Fatal("Tenant Owner role not found")
	}
	for _, permission := range []string{"subscriber.read.tenant", "subscriber.manage.tenant", "subscriber.import.tenant", "subscriber.export.tenant", "list.read.tenant", "list.manage.tenant"} {
		can, err := svc.HasTenantPermission(ctx, tenant.ID, 201, permission)
		if err != nil || !can {
			t.Fatalf("owner permission %s = %t, %v", permission, can, err)
		}
	}

	// --- Explicit deny beats grant ---
	can, err := svc.HasTenantPermission(ctx, tenant.ID, 201, "campaign.create.tenant")
	if err != nil || !can {
		t.Fatalf("expected grant before deny: %t, %v", can, err)
	}
	if err := svc.DenyRolePermission(ctx, tenant.ID, ownerRoleID, "campaign.create.tenant", actor); err != nil {
		t.Fatalf("DenyRolePermission: %v", err)
	}
	can, err = svc.HasTenantPermission(ctx, tenant.ID, 201, "campaign.create.tenant")
	if err != nil || can {
		t.Fatalf("expected denial to override grant: %t, %v", can, err)
	}
	if err := svc.AllowRolePermission(ctx, tenant.ID, ownerRoleID, "campaign.create.tenant", actor); err != nil {
		t.Fatalf("AllowRolePermission: %v", err)
	}
	can, err = svc.HasTenantPermission(ctx, tenant.ID, 201, "campaign.create.tenant")
	if err != nil || !can {
		t.Fatalf("expected grant restored after allow: %t, %v", can, err)
	}

	// --- Custom roles require a premium plan ---
	if _, err := svc.CreateTenantRole(ctx, tenant.ID, CreateTenantRoleInput{Name: "QA", PermissionCodes: []string{"campaign.read.tenant"}}, actor); err != ErrPlanDoesNotAllowCustomRoles {
		t.Fatalf("expected ErrPlanDoesNotAllowCustomRoles on starter plan, got %v", err)
	}
	if _, err := svc.SetTenantQuotaPlan(ctx, tenant.ID, "growth", actor); err != nil {
		t.Fatal(err)
	}
	role, err := svc.CreateTenantRole(ctx, tenant.ID, CreateTenantRoleInput{Name: "QA", PermissionCodes: []string{"campaign.read.tenant"}}, actor)
	if err != nil {
		t.Fatalf("CreateTenantRole on growth plan: %v", err)
	}
	if role.IsSystem {
		t.Fatal("custom role should not be marked system")
	}

	// --- Admin ops ---
	dash, err := svc.GetPlatformDashboard(ctx)
	if err != nil || dash.TenantsActive < 1 {
		t.Fatalf("GetPlatformDashboard: %#v, %v", dash, err)
	}
	infra, err := svc.SetTenantInfrastructureMode(ctx, tenant.ID, "dedicated_requested", actor)
	if err != nil || infra.Mode != "dedicated_requested" {
		t.Fatalf("SetTenantInfrastructureMode: %#v, %v", infra, err)
	}
	if _, err := svc.ResetTenantOwner(ctx, tenant.ID, 202, actor); err != nil {
		t.Fatalf("ResetTenantOwner: %v", err)
	}

	// --- Impersonation: MFA recency required ---
	_, err = svc.StartImpersonation(ctx, 201, StartImpersonationInput{TenantID: tenant.ID, TargetUserID: 202, Reason: "customer support ticket #123", TTLMinutes: 15}, actor)
	if err != ErrMFANotRecent {
		t.Fatalf("expected ErrMFANotRecent without TOTP, got %v", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO mv_mfa_methods (id, user_id, type, secret_ciphertext, last_used_at) VALUES (gen_random_uuid(), 201, 'totp', 'x', now())`); err != nil {
		t.Fatal(err)
	}

	// --- TTL bounds ---
	if _, err := svc.StartImpersonation(ctx, 201, StartImpersonationInput{TenantID: tenant.ID, TargetUserID: 202, Reason: "customer support ticket #123", TTLMinutes: 999}, actor); err != ErrInvalid {
		t.Fatalf("expected ErrInvalid for TTL over max, got %v", err)
	}
	if _, err := svc.StartImpersonation(ctx, 201, StartImpersonationInput{TenantID: tenant.ID, TargetUserID: 202, Reason: "short", TTLMinutes: 15}, actor); err != ErrInvalid {
		t.Fatalf("expected ErrInvalid for short reason, got %v", err)
	}

	grant, err := svc.StartImpersonation(ctx, 201, StartImpersonationInput{TenantID: tenant.ID, TargetUserID: 202, Reason: "customer support ticket #123", TTLMinutes: 15}, actor)
	if err != nil {
		t.Fatalf("StartImpersonation: %v", err)
	}
	if grant.ExpiresAt.Sub(grant.CreatedAt) > 16*time.Minute {
		t.Fatalf("grant TTL not honored: %#v", grant)
	}

	if _, err := svc.ValidateImpersonationGrant(ctx, grant.ID, 999); err != ErrGrantNotOwned {
		t.Fatalf("expected ErrGrantNotOwned for wrong actor, got %v", err)
	}
	valid, err := svc.ValidateImpersonationGrant(ctx, grant.ID, 201)
	if err != nil || valid.TargetUserID != 202 {
		t.Fatalf("ValidateImpersonationGrant: %#v, %v", valid, err)
	}

	revoked, err := svc.RevokeImpersonation(ctx, grant.ID, actor)
	if err != nil || revoked.RevokedAt == nil {
		t.Fatalf("RevokeImpersonation: %#v, %v", revoked, err)
	}
	if _, err := svc.ValidateImpersonationGrant(ctx, grant.ID, 201); err != ErrGrantExpired {
		t.Fatalf("expected ErrGrantExpired after revoke, got %v", err)
	}

	grants, err := svc.ListImpersonationGrants(ctx)
	if err != nil || len(grants) != 1 {
		t.Fatalf("ListImpersonationGrants = %d, %v", len(grants), err)
	}
}
