package tenant

import (
	"context"
	"os"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/mailview/migrations"
	_ "github.com/lib/pq"
)

func TestRLSIsolationIntegration(t *testing.T) {
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
	if _, err := db.ExecContext(ctx, `CREATE ROLE mv_rls_test NOINHERIT; GRANT USAGE ON SCHEMA public TO mv_rls_test; GRANT SELECT, INSERT, UPDATE, DELETE ON mv_tenant_settings TO mv_rls_test`); err != nil {
		t.Fatal(err)
	}
	first, second := uuid.Must(uuid.NewV4()), uuid.Must(uuid.NewV4())
	if _, err := db.ExecContext(ctx, `INSERT INTO mv_tenants (id, slug, name) VALUES ($1, 'tenant-one', 'Tenant One'), ($2, 'tenant-two', 'Tenant Two')`, first, second); err != nil {
		t.Fatal(err)
	}

	for _, tenantID := range []uuid.UUID{first, second} {
		scoped := WithContext(ctx, Context{TenantID: tenantID, UserID: 1, RequestID: "rls-test"})
		if err := InTransaction(scoped, db, func(tx *sqlx.Tx, _ Context) error {
			if _, err := tx.ExecContext(scoped, `SET LOCAL ROLE mv_rls_test`); err != nil {
				return err
			}
			_, err := tx.ExecContext(scoped, `INSERT INTO mv_tenant_settings (tenant_id, key, value) VALUES ($1, 'branding.name', '{"ok": true}')`, tenantID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, tenantID := range []uuid.UUID{first, second} {
		scoped := WithContext(ctx, Context{TenantID: tenantID})
		if err := InTransaction(scoped, db, func(tx *sqlx.Tx, _ Context) error {
			if _, err := tx.ExecContext(scoped, `SET LOCAL ROLE mv_rls_test`); err != nil {
				return err
			}
			var count int
			if err := tx.GetContext(scoped, &count, `SELECT count(*) FROM mv_tenant_settings`); err != nil {
				return err
			}
			if count != 1 {
				t.Fatalf("tenant %s read %d settings", tenantID, count)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	// A policy violation aborts the SQL transaction, so it is exercised in its
	// own transaction and must be returned to the caller.
	crossTenant := WithContext(ctx, Context{TenantID: first})
	err = InTransaction(crossTenant, db, func(tx *sqlx.Tx, _ Context) error {
		if _, err := tx.ExecContext(crossTenant, `SET LOCAL ROLE mv_rls_test`); err != nil {
			return err
		}
		_, err := tx.ExecContext(crossTenant, `INSERT INTO mv_tenant_settings (tenant_id, key, value) VALUES ($1, 'forbidden', '{}')`, second)
		return err
	})
	if err == nil {
		t.Fatal("cross-tenant insert was allowed")
	}
}
