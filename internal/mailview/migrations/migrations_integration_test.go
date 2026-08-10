package migrations

import (
	"context"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// TestUpgradeIntegration is opt-in because it needs a PostgreSQL instance with
// the upstream Listmonk schema already installed. It validates the actual SQL,
// migration ledger, and append-only audit trigger used in production.
func TestUpgradeIntegration(t *testing.T) {
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
	if err := Upgrade(ctx, db); err != nil {
		t.Fatal(err)
	}
	// A second run must be a no-op.
	if err := Upgrade(ctx, db); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.GetContext(ctx, &count, `SELECT count(*) FROM mv_schema_migrations`); err != nil || count != len(all) {
		t.Fatalf("migration ledger count = %d, %v", count, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO mv_audit_events (id, action, resource_type, resource_id, result) VALUES ('00000000-0000-0000-0000-000000000001', 'test.create', 'test', '1', 'success')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM mv_audit_events WHERE id = '00000000-0000-0000-0000-000000000001'`); err == nil {
		t.Fatal("append-only audit trigger allowed delete")
	}
	var protected int
	if err := db.GetContext(ctx, &protected, `
SELECT count(*) FROM pg_class
WHERE relname = ANY($1) AND relrowsecurity AND relforcerowsecurity`,
		pq.Array([]string{"subscribers", "lists", "subscriber_lists", "templates", "campaigns", "campaign_lists", "campaign_views", "media", "campaign_media", "links", "link_clicks", "bounces"})); err != nil {
		t.Fatal(err)
	}
	if protected != 12 {
		t.Fatalf("FORCE RLS active on %d/12 tenant Data Plane tables", protected)
	}
	phase3Tables := []string{
		"mv_tenant_branding", "mv_tenant_sessions", "mv_api_keys", "mv_billing_accounts", "mv_subscriptions", "mv_invoices", "mv_feature_flags",
		"mv_smtp_profiles", "mv_sender_identities", "mv_sending_domains", "mv_domain_dns_records", "mv_complaints", "mv_campaign_events",
		"mv_exports", "mv_webhooks", "mv_webhook_deliveries", "mv_transactional_messages",
	}
	if err := db.GetContext(ctx, &protected, `SELECT count(*) FROM pg_class WHERE relname=ANY($1) AND relrowsecurity AND relforcerowsecurity`, pq.Array(phase3Tables)); err != nil {
		t.Fatal(err)
	}
	if protected != len(phase3Tables) {
		t.Fatalf("FORCE RLS active on %d/%d Phase 3 tables", protected, len(phase3Tables))
	}
	phase4Tables := []string{"mv_campaign_workflows", "mv_campaign_workflow_events"}
	if err := db.GetContext(ctx, &protected, `SELECT count(*) FROM pg_class WHERE relname=ANY($1) AND relrowsecurity AND relforcerowsecurity`, pq.Array(phase4Tables)); err != nil {
		t.Fatal(err)
	}
	if protected != len(phase4Tables) {
		t.Fatalf("FORCE RLS active on %d/%d Phase 4 tables", protected, len(phase4Tables))
	}
}
