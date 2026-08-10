// Package job defines the signed envelope required by every asynchronous
// MailView producer/consumer. Tenant identity is part of the signature and is
// never accepted from a mutable payload field after verification.
package job

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

var (
	ErrInvalidKey      = errors.New("job signing key must decode to 32 bytes")
	ErrInvalidEnvelope = errors.New("invalid signed job envelope")
)

type Signer struct {
	key []byte
	now func() time.Time
}

type Envelope struct {
	Version   int             `json:"version"`
	JobID     uuid.UUID       `json:"job_id"`
	TenantID  uuid.UUID       `json:"tenant_id"`
	Type      string          `json:"type"`
	IssuedAt  time.Time       `json:"issued_at"`
	ExpiresAt time.Time       `json:"expires_at"`
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

func NewSigner(encodedKey string) (*Signer, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return &Signer{key: key, now: time.Now}, nil
}

func (s *Signer) Sign(jobID, tenantID uuid.UUID, jobType string, payload any, ttl time.Duration) (Envelope, error) {
	jobType = strings.TrimSpace(jobType)
	if jobID == uuid.Nil || tenantID == uuid.Nil || jobType == "" || ttl <= 0 || ttl > 7*24*time.Hour {
		return Envelope{}, ErrInvalidEnvelope
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	now := s.now().UTC()
	out := Envelope{Version: 1, JobID: jobID, TenantID: tenantID, Type: jobType, IssuedAt: now, ExpiresAt: now.Add(ttl), Payload: raw}
	out.Signature = s.signature(out)
	return out, nil
}

// Verify authenticates all routing claims before returning tenant_id. Callers
// must use this returned UUID to open tenant.Begin; payload tenant fields are
// deliberately irrelevant.
func (s *Signer) Verify(envelope Envelope) (uuid.UUID, error) {
	if envelope.Version != 1 || envelope.JobID == uuid.Nil || envelope.TenantID == uuid.Nil || strings.TrimSpace(envelope.Type) == "" {
		return uuid.Nil, ErrInvalidEnvelope
	}
	now := s.now().UTC()
	if envelope.ExpiresAt.Before(now) || envelope.IssuedAt.After(now.Add(5*time.Minute)) || envelope.ExpiresAt.Sub(envelope.IssuedAt) > 7*24*time.Hour {
		return uuid.Nil, ErrInvalidEnvelope
	}
	expected, err := hex.DecodeString(s.signature(envelope))
	if err != nil {
		return uuid.Nil, ErrInvalidEnvelope
	}
	actual, err := hex.DecodeString(envelope.Signature)
	if err != nil || !hmac.Equal(actual, expected) {
		return uuid.Nil, ErrInvalidEnvelope
	}
	return envelope.TenantID, nil
}

func (s *Signer) signature(envelope Envelope) string {
	payloadHash := sha256.Sum256(envelope.Payload)
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(envelope.JobID.String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(envelope.TenantID.String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(envelope.Type))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(envelope.IssuedAt.UTC().Format(time.RFC3339Nano)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(envelope.ExpiresAt.UTC().Format(time.RFC3339Nano)))
	_, _ = mac.Write(payloadHash[:])
	return hex.EncodeToString(mac.Sum(nil))
}
