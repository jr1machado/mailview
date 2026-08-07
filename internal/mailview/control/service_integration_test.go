package control

import (
	"context"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/mailview/migrations"
	_ "github.com/lib/pq"
)

// TestControlPlaneIntegration is opt-in and runs against the same temporary
// PostgreSQL used by the migration integration test.
func TestControlPlaneIntegration(t *testing.T) {
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
	if _, err := db.ExecContext(ctx, `INSERT INTO roles (id, type, name, permissions) VALUES (1, 'user', 'Super Admin', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_login, password, email, name, type, user_role_id, status) VALUES (1, 'owner', true, 'unused', 'owner@example.test', 'Owner', 'user', 1, 'enabled')`); err != nil {
		t.Fatal(err)
	}

	svc := New(db)
	tenant, err := svc.CreateTenant(ctx, CreateTenantInput{Slug: "Acme-Email", Name: "Acme Email", OwnerUserID: 1}, Actor{UserID: 1, RequestID: "test-request"})
	if err != nil {
		t.Fatal(err)
	}
	if tenant.Slug != "acme-email" || tenant.Status != TenantStatusActive {
		t.Fatalf("unexpected tenant: %#v", tenant)
	}
	roles, err := svc.ListRoles(ctx, tenant.ID)
	if err != nil || len(roles) != 7 {
		t.Fatalf("roles = %d, %v", len(roles), err)
	}
	memberships, err := svc.ListMemberships(ctx, tenant.ID)
	if err != nil || len(memberships) != 1 || memberships[0].UserID != 1 {
		t.Fatalf("memberships = %#v, %v", memberships, err)
	}
	if _, err := svc.UpdateTenantStatus(ctx, tenant.ID, TenantStatusSuspended, Actor{UserID: 1}); err != nil {
		t.Fatal(err)
	}
	events, err := svc.ListAuditEvents(ctx, tenant.ID, 10)
	if err != nil || len(events) != 2 {
		t.Fatalf("events = %d, %v", len(events), err)
	}
	codes, err := svc.GenerateRecoveryCodes(ctx, 1, Actor{UserID: 1})
	if err != nil || len(codes) != 10 {
		t.Fatalf("recovery codes = %d, %v", len(codes), err)
	}
	var stored int
	if err := db.GetContext(ctx, &stored, `SELECT count(*) FROM mv_mfa_recovery_codes WHERE user_id = 1`); err != nil || stored != 10 {
		t.Fatalf("stored recovery codes = %d, %v", stored, err)
	}
	used, err := svc.UseRecoveryCode(ctx, 1, codes[0], Actor{UserID: 1})
	if err != nil || !used {
		t.Fatalf("use recovery code = %t, %v", used, err)
	}
	used, err = svc.UseRecoveryCode(ctx, 1, codes[0], Actor{UserID: 1})
	if err != nil || used {
		t.Fatalf("reused recovery code = %t, %v", used, err)
	}
}
