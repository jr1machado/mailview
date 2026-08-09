package filesystem

import (
	"bytes"
	"testing"
)

func TestTenantMediaPathRoundTrip(t *testing.T) {
	dir := t.TempDir()
	client := &Client{opts: Opts{
		UploadPath: dir,
		UploadURI:  "/uploads",
		RootURL:    "https://mail.example.test",
	}}
	name := "00000000-0000-0000-0000-000000000001/image.png"
	payload := []byte("tenant image")

	if _, err := client.Put(name, "image/png", bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}

	for _, rawURL := range []string{
		client.GetURL(name),
		"/uploads/" + name,
		name,
	} {
		got, err := client.GetBlob(rawURL)
		if err != nil {
			t.Fatalf("GetBlob(%q): %v", rawURL, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("GetBlob(%q) = %q, want %q", rawURL, got, payload)
		}
	}
}

func TestTenantMediaPathRejectsTraversal(t *testing.T) {
	client := &Client{opts: Opts{UploadPath: t.TempDir(), UploadURI: "/uploads"}}
	for _, rawURL := range []string{"../secret", "/uploads/../../secret", "https://example.test/uploads/../../secret"} {
		if _, err := client.GetBlob(rawURL); err == nil {
			t.Fatalf("GetBlob(%q) accepted a path outside the upload directory", rawURL)
		}
		if _, err := client.Put(rawURL, "text/plain", bytes.NewReader(nil)); err == nil {
			t.Fatalf("Put(%q) accepted a path outside the upload directory", rawURL)
		}
		if err := client.Delete(rawURL); err == nil {
			t.Fatalf("Delete(%q) accepted a path outside the upload directory", rawURL)
		}
	}
}
