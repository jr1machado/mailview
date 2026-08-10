package job

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
)

func TestSignedEnvelopeBindsTenantAndPayload(t *testing.T) {
	signer, err := NewSigner(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	signer.now = func() time.Time { return now }
	tenantID := uuid.Must(uuid.NewV4())
	envelope, err := signer.Sign(uuid.Must(uuid.NewV4()), tenantID, "export.generate", map[string]any{"format": "csv"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := signer.Verify(envelope)
	if err != nil || resolved != tenantID {
		t.Fatalf("Verify = %s, %v", resolved, err)
	}

	tampered := envelope
	tampered.TenantID = uuid.Must(uuid.NewV4())
	if _, err := signer.Verify(tampered); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("tampered tenant accepted: %v", err)
	}
	tampered = envelope
	tampered.Payload = []byte(`{"format":"json"}`)
	if _, err := signer.Verify(tampered); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("tampered payload accepted: %v", err)
	}
}
