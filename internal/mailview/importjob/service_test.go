package importjob

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/mailview/tenant"
)

func TestWorkerEnvelopeRejectsTenantTampering(t *testing.T) {
	svc, err := New(nil, t.TempDir(), base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithContext(context.Background(), tenant.Context{TenantID: uuid.Must(uuid.NewV4()), UserID: 7})
	envelope, err := svc.WorkerEnvelope(ctx, uuid.Must(uuid.NewV4()))
	if err != nil {
		t.Fatal(err)
	}
	envelope.TenantID = uuid.Must(uuid.NewV4())
	if err := svc.ProcessEnvelope(context.Background(), envelope); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered worker envelope accepted: %v", err)
	}
}
