package tenant

import (
	"context"
	"testing"

	"github.com/gofrs/uuid/v5"
)

func TestContextRoundTrip(t *testing.T) {
	value := Context{TenantID: uuid.Must(uuid.NewV4()), UserID: 7, RequestID: "request-1"}
	got, ok := FromContext(WithContext(context.Background(), value))
	if !ok || got != value {
		t.Fatalf("context = %#v, %t", got, ok)
	}
}

func TestContextRejectsMissingTenant(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("missing context was accepted")
	}
}
