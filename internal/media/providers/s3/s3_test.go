package s3

import "testing"

func TestObjectNamePreservesTenantPrefix(t *testing.T) {
	client := &Client{opts: Opt{
		URL:        "https://s3.example.test",
		PublicURL:  "/uploads",
		Bucket:     "mailview",
		BucketPath: "media/root",
	}}
	want := "11111111-1111-1111-1111-111111111111/image.png"
	for _, raw := range []string{
		want,
		"media/root/" + want,
		"/uploads/media/root/" + want,
		"https://mail.example.test/uploads/media/root/" + want,
		"https://s3.example.test/mailview/media/root/" + want + "?signature=test",
	} {
		got, err := client.objectName(raw)
		if err != nil {
			t.Fatalf("objectName(%q): %v", raw, err)
		}
		if got != want {
			t.Fatalf("objectName(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestObjectNameRejectsInvalidPath(t *testing.T) {
	client := &Client{}
	for _, raw := range []string{"", ".", "..", "\\..\\secret"} {
		if _, err := client.objectName(raw); err == nil {
			t.Fatalf("objectName(%q) accepted invalid path", raw)
		}
	}
}
