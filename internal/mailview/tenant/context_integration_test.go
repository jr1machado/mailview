package tenant

import (
	"context"
	"fmt"
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
	if _, err := db.ExecContext(ctx, `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='mv_rls_test') THEN CREATE ROLE mv_rls_test NOINHERIT; END IF; END $$; GRANT USAGE ON SCHEMA public TO mv_rls_test; GRANT SELECT, INSERT, UPDATE, DELETE ON mv_tenant_settings, subscribers, lists, subscriber_lists, templates, campaigns, campaign_lists, campaign_views, media, campaign_media, links, link_clicks, bounces TO mv_rls_test; GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO mv_rls_test`); err != nil {
		t.Fatal(err)
	}
	first, second := uuid.Must(uuid.NewV4()), uuid.Must(uuid.NewV4())
	if _, err := db.ExecContext(ctx, `INSERT INTO mv_tenants (id, slug, name) VALUES ($1, $2, 'Tenant One'), ($3, $4, 'Tenant Two')`,
		first, "tenant-one-"+first.String()[:8], second, "tenant-two-"+second.String()[:8]); err != nil {
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

	for i, tenantID := range []uuid.UUID{first, second} {
		scoped := WithContext(ctx, Context{TenantID: tenantID})
		if err := InTransaction(scoped, db, func(tx *sqlx.Tx, _ Context) error {
			if _, err := tx.ExecContext(scoped, `SET LOCAL ROLE mv_rls_test`); err != nil {
				return err
			}
			var templateID, subscriberID, listID, campaignID, mediaID, linkID int
			if err := tx.GetContext(scoped, &templateID, `INSERT INTO templates(name,subject,body) VALUES($1,'Subject','Body') RETURNING id`, "Template "+string(rune('A'+i))); err != nil {
				return err
			}
			subscriberEmail := "shared@example.test"
			if err := tx.GetContext(scoped, &subscriberID, `INSERT INTO subscribers(uuid,email,name,status) VALUES($1,$2,'RLS','enabled') ON CONFLICT (tenant_id,(lower(email))) DO UPDATE SET name=EXCLUDED.name RETURNING id`, uuid.Must(uuid.NewV4()), subscriberEmail); err != nil {
				return err
			}
			var existingSubscriberID int
			if err := tx.GetContext(scoped, &existingSubscriberID, `INSERT INTO subscribers(uuid,email,name,status) VALUES($1,$2,'RLS','enabled') ON CONFLICT (tenant_id,(lower(email))) DO UPDATE SET name=EXCLUDED.name RETURNING id`, uuid.Must(uuid.NewV4()), subscriberEmail); err != nil {
				return err
			}
			if existingSubscriberID != subscriberID {
				return fmt.Errorf("subscriber upsert returned %d, want %d", existingSubscriberID, subscriberID)
			}
			if err := tx.GetContext(scoped, &listID, `INSERT INTO lists(uuid,name,type,optin,status,description) VALUES(gen_random_uuid(),$1,'private','single','active','') RETURNING id`, "RLS list "+string(rune('A'+i))); err != nil {
				return err
			}
			if _, err := tx.ExecContext(scoped, `INSERT INTO subscriber_lists(subscriber_id,list_id,status) VALUES($1,$2,'confirmed')`, subscriberID, listID); err != nil {
				return err
			}
			if err := tx.GetContext(scoped, &mediaID, `INSERT INTO media(uuid,filename,content_type,thumb) VALUES(gen_random_uuid(),$1,'image/png','') RETURNING id`, fmt.Sprintf("%s/image.png", tenantID)); err != nil {
				return err
			}
			if err := tx.GetContext(scoped, &campaignID, `INSERT INTO campaigns(uuid,name,subject,from_email,body,messenger,template_id) VALUES(gen_random_uuid(),$1,'Subject','from@example.test','Body','email',$2) RETURNING id`, "Campaign "+string(rune('A'+i)), templateID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(scoped, `INSERT INTO campaign_lists(campaign_id,list_id,list_name) VALUES($1,$2,'RLS list')`, campaignID, listID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(scoped, `INSERT INTO campaign_views(campaign_id,subscriber_id) VALUES($1,$2)`, campaignID, subscriberID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(scoped, `INSERT INTO campaign_media(campaign_id,media_id,filename) VALUES($1,$2,'image.png')`, campaignID, mediaID); err != nil {
				return err
			}
			linkURL := "https://example.test/shared"
			if err := tx.GetContext(scoped, &linkID, `INSERT INTO links(uuid,url) VALUES($1,$2) ON CONFLICT (tenant_id,url) DO UPDATE SET url=EXCLUDED.url RETURNING id`, uuid.Must(uuid.NewV4()), linkURL); err != nil {
				return err
			}
			var existingLinkID int
			if err := tx.GetContext(scoped, &existingLinkID, `INSERT INTO links(uuid,url) VALUES($1,$2) ON CONFLICT (tenant_id,url) DO UPDATE SET url=EXCLUDED.url RETURNING id`, uuid.Must(uuid.NewV4()), linkURL); err != nil {
				return err
			}
			if existingLinkID != linkID {
				return fmt.Errorf("link upsert returned %d, want %d", existingLinkID, linkID)
			}
			if _, err := tx.ExecContext(scoped, `INSERT INTO link_clicks(campaign_id,link_id,subscriber_id) VALUES($1,$2,$3)`, campaignID, linkID, subscriberID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(scoped, `INSERT INTO bounces(subscriber_id,campaign_id,type) VALUES($1,$2,'hard')`, subscriberID, campaignID); err != nil {
				return err
			}
			return nil
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
			for _, table := range []string{"subscribers", "lists", "subscriber_lists", "templates", "campaigns", "campaign_lists", "campaign_views", "media", "campaign_media", "links", "link_clicks", "bounces"} {
				var count int
				if err := tx.GetContext(scoped, &count, `SELECT count(*) FROM `+table); err != nil {
					return err
				}
				if count != 1 {
					t.Fatalf("tenant %s read %d rows from %s", tenantID, count, table)
				}
			}
			return nil
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

	// Context-free access is confined to the legacy workspace.
	for _, tenantID := range []uuid.UUID{first, second} {
		scoped := WithContext(ctx, Context{TenantID: tenantID})
		if err := InTransaction(scoped, db, func(tx *sqlx.Tx, _ Context) error {
			if _, err := tx.ExecContext(scoped, `SET LOCAL ROLE mv_rls_test`); err != nil {
				return err
			}
			var count int
			if err := tx.GetContext(scoped, &count, `SELECT count(*) FROM lists`); err != nil {
				return err
			}
			if count != 1 {
				t.Fatalf("tenant %s read %d lists", tenantID, count)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	var unscopedCount int
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE mv_rls_test`); err != nil {
		t.Fatal(err)
	}
	if err := tx.GetContext(ctx, &unscopedCount, `SELECT count(*) FROM lists WHERE tenant_id <> '00000000-0000-0000-0000-000000000001'`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if unscopedCount != 0 {
		t.Fatalf("context-free role saw %d SaaS tenant lists", unscopedCount)
	}
}
