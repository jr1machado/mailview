package control

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

var ErrMFAEncryptionUnavailable = errors.New("MFA encryption key is not configured")

// MFA stores newly enrolled TOTP secrets outside Listmonk's legacy plaintext
// users.twofa_key column. Empty configuration preserves existing accounts but
// refuses new enrollment until a 256-bit key is configured.
type MFA struct {
	db   *sqlx.DB
	aead cipher.AEAD
}

func NewMFA(db *sqlx.DB, encodedKey string) (*MFA, error) {
	m := &MFA{db: db}
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return m, nil
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("decode MailView MFA encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("MailView MFA encryption key must decode to 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	m.aead = aead
	return m, nil
}

func (m *MFA) EncryptionReady() bool { return m != nil && m.aead != nil }

func (m *MFA) EnableTOTP(ctx context.Context, userID int, secret string, actor Actor) error {
	if !m.EncryptionReady() {
		return ErrMFAEncryptionUnavailable
	}
	if userID < 1 || strings.TrimSpace(secret) == "" {
		return ErrInvalid
	}
	ciphertext, err := m.encrypt([]byte(secret))
	if err != nil {
		return err
	}
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `
INSERT INTO mv_mfa_methods (id, user_id, type, secret_ciphertext, key_version)
VALUES ($1, $2, 'totp', $3, 1)
ON CONFLICT (user_id, type) DO UPDATE
SET secret_ciphertext = EXCLUDED.secret_ciphertext, key_version = EXCLUDED.key_version, enabled_at = now(), last_used_at = NULL`, uuid.Must(uuid.NewV4()), userID, ciphertext); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET twofa_type = 'totp', twofa_key = NULL, updated_at = now() WHERE id = $1`, userID); err != nil {
		return err
	}
	if err = appendAudit(ctx, tx, nil, actor, "mfa.totp.enable", "user", fmt.Sprintf("%d", userID), "success", "", nil); err != nil {
		return err
	}
	return tx.Commit()
}

// TOTPSecret returns found=false for legacy enrollments. Callers may use the
// legacy value temporarily until the account re-enrolls with an encryption key.
func (m *MFA) TOTPSecret(ctx context.Context, userID int) (secret string, found bool, err error) {
	if !m.EncryptionReady() {
		return "", false, nil
	}
	var ciphertext []byte
	if err := m.db.GetContext(ctx, &ciphertext, `SELECT secret_ciphertext FROM mv_mfa_methods WHERE user_id = $1 AND type = 'totp'`, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	plain, err := m.decrypt(ciphertext)
	if err != nil {
		return "", false, err
	}
	return string(plain), true, nil
}

func (m *MFA) DisableTOTP(ctx context.Context, userID int, actor Actor) error {
	if m == nil {
		return errors.New("MFA service is unavailable")
	}
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mv_mfa_methods WHERE user_id = $1 AND type = 'totp'`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET twofa_type = 'none', twofa_key = NULL, updated_at = now() WHERE id = $1`, userID); err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, nil, actor, "mfa.totp.disable", "user", fmt.Sprintf("%d", userID), "success", "", nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *MFA) encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return m.aead.Seal(nonce, nonce, plain, nil), nil
}

func (m *MFA) decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < m.aead.NonceSize() {
		return nil, errors.New("invalid encrypted MFA secret")
	}
	nonce, data := ciphertext[:m.aead.NonceSize()], ciphertext[m.aead.NonceSize():]
	return m.aead.Open(nil, nonce, data, nil)
}
