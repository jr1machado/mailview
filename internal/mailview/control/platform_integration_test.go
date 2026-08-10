package control

import (
	"context"
	"os"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/mailview/migrations"
	_ "github.com/lib/pq"
)

// TestPlatformAndDomainsIntegration is opt-in and covers the items added on
// top of Phase 1/2: platform RBAC assignment/revocation, permission checks,
// tenant domain hostname uniqueness across tenants, and quota plan
// assignment.
func TestPlatformAndDomainsIntegration(t *testing.T) {
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
	// IDs are offset from the sibling TestControlPlaneIntegration fixture (role/user 1)
	// so both tests in this package can run in the same `go test` invocation.
	if _, err := db.ExecContext(ctx, `INSERT INTO roles (id, type, name, permissions) VALUES (2, 'user', 'Super Admin 2', '{}') ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, username, password_login, password, email, name, type, user_role_id, status) VALUES
    (101, 'owner2', true, 'unused', 'owner2@example.test', 'Owner2', 'user', 2, 'enabled'),
    (102, 'ops2', true, 'unused', 'ops2@example.test', 'Ops2', 'user', 2, 'enabled')
ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}

	svc := NewWithDNSResolver(db, fakeDNSResolver{})
	actor := Actor{UserID: 101, RequestID: "test-request"}

	if err := svc.EnsureNoBypassRLS(ctx); err != nil {
		t.Fatalf("EnsureNoBypassRLS: %v", err)
	}

	// Platform RBAC: seeded roles, assignment, permission check, revocation.
	roles, err := svc.ListPlatformRoles(ctx)
	if err != nil || len(roles) != 6 {
		t.Fatalf("platform roles = %d, %v", len(roles), err)
	}
	var opsRoleID uuid.UUID
	for _, r := range roles {
		if r.Name == "Platform Operations" {
			opsRoleID = r.ID
		}
	}
	if opsRoleID == uuid.Nil {
		t.Fatal("Platform Operations role not found")
	}

	if _, err := svc.AssignPlatformRole(ctx, 102, opsRoleID, actor); err != nil {
		t.Fatalf("AssignPlatformRole: %v", err)
	}
	can, err := svc.HasPlatformPermission(ctx, 102, "tenant.manage.platform")
	if err != nil || !can {
		t.Fatalf("HasPlatformPermission tenant.manage.platform = %t, %v", can, err)
	}
	can, err = svc.HasPlatformPermission(ctx, 102, "platform.roles.manage")
	if err != nil || can {
		t.Fatalf("Platform Operations should not have platform.roles.manage: %t, %v", can, err)
	}
	permissions, err := svc.ListPlatformPermissions(ctx, 102)
	if err != nil || len(permissions) == 0 {
		t.Fatalf("ListPlatformPermissions = %v, %v", permissions, err)
	}
	foundTenantManage := false
	for _, permission := range permissions {
		if permission == "tenant.manage.platform" {
			foundTenantManage = true
		}
		if permission == "platform.roles.manage" {
			t.Fatal("Platform Operations unexpectedly exposes platform.roles.manage")
		}
	}
	if !foundTenantManage {
		t.Fatal("Platform Operations does not expose tenant.manage.platform")
	}
	assignments, err := svc.ListPlatformAssignments(ctx)
	if err != nil || len(assignments) != 1 {
		t.Fatalf("assignments = %d, %v", len(assignments), err)
	}
	if err := svc.RevokePlatformRole(ctx, 102, opsRoleID, actor); err != nil {
		t.Fatalf("RevokePlatformRole: %v", err)
	}
	can, err = svc.HasPlatformPermission(ctx, 102, "tenant.manage.platform")
	if err != nil || can {
		t.Fatalf("permission should be gone after revoke: %t, %v", can, err)
	}

	// Tenant domains: create two tenants, hostname must be globally unique.
	tenantA, err := svc.CreateTenant(ctx, CreateTenantInput{Slug: "acme-a", Name: "Acme A", OwnerUserID: 101}, actor)
	if err != nil {
		t.Fatal(err)
	}
	tenantB, err := svc.CreateTenant(ctx, CreateTenantInput{Slug: "acme-b", Name: "Acme B", OwnerUserID: 101}, actor)
	if err != nil {
		t.Fatal(err)
	}
	slugChange, err := svc.ChangeTenantSlug(ctx, tenantB.ID, ChangeTenantSlugInput{Slug: "acme-renamed", RedirectDays: 7}, actor)
	if err != nil || slugChange.Tenant.Slug != "acme-renamed" {
		t.Fatalf("ChangeTenantSlug: %#v, %v", slugChange, err)
	}
	resolvedAlias, canonical, err := svc.ResolveTenantSlug(ctx, "acme-b")
	if err != nil || canonical || resolvedAlias.ID != tenantB.ID {
		t.Fatalf("ResolveTenantSlug alias: %#v, canonical=%t, %v", resolvedAlias, canonical, err)
	}

	domain, err := svc.CreateTenantDomain(ctx, tenantA.ID, CreateTenantDomainInput{Hostname: "mail.acme.example", Purpose: "sending"}, actor)
	if err != nil {
		t.Fatalf("CreateTenantDomain: %v", err)
	}
	if domain.Status != "pending" || domain.TLSStatus != "none" || domain.VerificationToken == "" {
		t.Fatalf("unexpected domain: %#v", domain)
	}
	if _, err := svc.CreateTenantDomain(ctx, tenantB.ID, CreateTenantDomainInput{Hostname: "mail.acme.example", Purpose: "sending"}, actor); err == nil {
		t.Fatal("cross-tenant hostname reuse was allowed")
	}
	svc.resolver = fakeDNSResolver{txt: map[string][]string{domain.VerificationName: {domain.VerificationValue}}}

	verified, err := svc.MarkTenantDomainVerified(ctx, tenantA.ID, domain.ID, actor)
	if err != nil || verified.Status != "verified" || verified.LastVerifiedAt == nil {
		t.Fatalf("MarkTenantDomainVerified: %#v, %v", verified, err)
	}
	resolved, err := svc.GetTenantByVerifiedHostname(ctx, domain.Hostname)
	if err != nil || resolved.ID != tenantA.ID {
		t.Fatalf("GetTenantByVerifiedHostname: %#v, %v", resolved, err)
	}

	domains, err := svc.ListTenantDomains(ctx, tenantA.ID)
	if err != nil || len(domains) != 1 {
		t.Fatalf("ListTenantDomains = %d, %v", len(domains), err)
	}

	// Quotas: default plan on first read, then switch plan.
	quota, err := svc.GetTenantQuota(ctx, tenantB.ID)
	if err != nil || quota.PlanCode != "starter" {
		t.Fatalf("default quota = %#v, %v", quota, err)
	}
	quota, err = svc.SetTenantQuotaPlan(ctx, tenantB.ID, "growth", actor)
	if err != nil || quota.PlanCode != "growth" {
		t.Fatalf("SetTenantQuotaPlan = %#v, %v", quota, err)
	}

	plans, err := svc.ListTenantPlans(ctx)
	if err != nil || len(plans) != 3 {
		t.Fatalf("plans = %d, %v", len(plans), err)
	}

	if _, err := svc.ConfigureTenantInfrastructure(ctx, tenantB.ID, TenantInfrastructureInput{Mode: "dedicated"}, actor); err == nil {
		t.Fatal("incomplete dedicated route was accepted")
	}
	route, err := svc.ConfigureTenantInfrastructure(ctx, tenantB.ID, TenantInfrastructureInput{
		Mode: "dedicated", DatabaseRef: "secret/db-acme", WorkerRef: "queue/acme", SMTPRef: "secret/smtp-acme",
		StorageRef: "bucket/acme", EncryptionKeyRef: "kms/acme", DockerNamespace: "mailview-acme",
	}, actor)
	if err != nil || route.Mode != "dedicated" || route.RoutingVersion < 1 || route.ActivatedAt == nil {
		t.Fatalf("ConfigureTenantInfrastructure = %#v, %v", route, err)
	}
}
