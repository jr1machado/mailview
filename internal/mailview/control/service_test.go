package control

import "testing"

func TestNormalizeSlug(t *testing.T) {
	tests := []struct {
		in, want string
		valid    bool
	}{
		{"Acme-News", "acme-news", true}, {"ab", "", false}, {"acme_news", "", false}, {"-acme", "", false}, {"acme-", "", false},
	}
	for _, tt := range tests {
		got, err := NormalizeSlug(tt.in)
		if tt.valid && (err != nil || got != tt.want) {
			t.Fatalf("NormalizeSlug(%q) = %q, %v", tt.in, got, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("NormalizeSlug(%q) accepted invalid input", tt.in)
		}
	}
}

func TestRecoveryCodeHasStableUserFormat(t *testing.T) {
	code, err := newRecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 17 || code[5] != '-' || code[11] != '-' {
		t.Fatalf("unexpected recovery code format: %q", code)
	}
}

func TestMFAEncryptionRoundTrip(t *testing.T) {
	key := "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	mfa, err := NewMFA(nil, key)
	if err != nil {
		t.Fatal(err)
	}
	if !mfa.EncryptionReady() {
		t.Fatal("expected encryption to be ready")
	}
	ciphertext, err := mfa.encrypt([]byte("totp-secret"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := mfa.decrypt(ciphertext)
	if err != nil || string(plain) != "totp-secret" {
		t.Fatalf("decrypt = %q, %v", plain, err)
	}
}
