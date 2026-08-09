package media

import "testing"

func TestValidateTenantPath(t *testing.T) {
	tenantID := "11111111-1111-1111-1111-111111111111"
	for _, key := range []string{
		tenantID + "/image.png",
		"bucket/prefix/" + tenantID + "/thumb_image.png",
	} {
		if got, err := ValidateTenantPath(key, tenantID); err != nil || got != key {
			t.Fatalf("ValidateTenantPath(%q) = %q, %v", key, got, err)
		}
	}

	for _, key := range []string{
		"22222222-2222-2222-2222-222222222222/image.png",
		"22222222-2222-2222-2222-222222222222/" + tenantID + "/image.png",
		"../22222222-2222-2222-2222-222222222222/image.png",
		"\\..\\secret",
	} {
		if _, err := ValidateTenantPath(key, tenantID); err == nil {
			t.Fatalf("ValidateTenantPath(%q) allowed cross-tenant media", key)
		}
	}
}
