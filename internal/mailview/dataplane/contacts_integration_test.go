package dataplane

import (
	"context"
	"os"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/mailview/migrations"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	_ "github.com/lib/pq"
)

func TestContactsAndListsIsolationIntegration(t *testing.T) {
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
	a, b := uuid.Must(uuid.NewV4()), uuid.Must(uuid.NewV4())
	if _, err := db.Exec(`INSERT INTO mv_tenants (id,slug,name) VALUES ($1,'data-tenant-a','Data A'),($2,'data-tenant-b','Data B')`, a, b); err != nil {
		t.Fatal(err)
	}
	svc := New(db)
	ctxA := tenant.WithContext(ctx, tenant.Context{TenantID: a})
	ctxB := tenant.WithContext(ctx, tenant.Context{TenantID: b})
	listA, err := svc.CreateList(ctxA, CreateListInput{Name: "A list"})
	if err != nil {
		t.Fatal(err)
	}
	listB, err := svc.CreateList(ctxB, CreateListInput{Name: "B list"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSubscriber(ctxA, CreateSubscriberInput{Email: "a@example.test", Name: "A", ListIDs: []int{listA.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSubscriber(ctxB, CreateSubscriberInput{Email: "b@example.test", Name: "B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSubscriber(ctxA, CreateSubscriberInput{Email: "bad@example.test", Name: "Bad", ListIDs: []int{listB.ID}}); err == nil {
		t.Fatal("cross-tenant list attachment was allowed")
	}
	subA, err := svc.CreateSubscriber(ctxA, CreateSubscriberInput{Email: "batch-a@example.test", Name: "Batch A"})
	if err != nil {
		t.Fatal(err)
	}
	subB, err := svc.CreateSubscriber(ctxB, CreateSubscriberInput{Email: "batch-b@example.test", Name: "Batch B"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.BlocklistSubscribers(ctxA, []int{subA.ID, subB.ID}); err == nil {
		t.Fatal("cross-tenant bulk blocklist was allowed")
	}
	gotA, err := svc.GetSubscriber(ctxA, subA.ID)
	if err != nil || gotA.Status != "enabled" {
		t.Fatalf("failed bulk operation was not atomic: %#v, %v", gotA, err)
	}
	if err := svc.ManageSubscriberLists(ctxA, []int{subA.ID}, []int{listB.ID}, "add", "confirmed"); err == nil {
		t.Fatal("cross-tenant bulk list attachment was allowed")
	}
	if err := svc.ManageSubscriberLists(ctxA, []int{subA.ID}, []int{listA.ID}, "add", "confirmed"); err != nil {
		t.Fatalf("same-tenant bulk list attachment failed: %v", err)
	}
	subs, err := svc.ListSubscribers(ctxA)
	if err != nil || len(subs) != 2 {
		t.Fatalf("tenant A subscribers=%#v err=%v", subs, err)
	}
	lists, err := svc.ListLists(ctxB)
	if err != nil || len(lists) != 1 || lists[0].Name != "B list" {
		t.Fatalf("tenant B lists=%#v err=%v", lists, err)
	}
}
