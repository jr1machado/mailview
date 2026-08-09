package media

import (
	"fmt"
	"path"
	"strings"

	"github.com/gofrs/uuid/v5"
)

// ValidateTenantPath normalizes a URL-style media key and, when tenantID is
// set, requires the physical tenant prefix to be present as a complete path
// component. Prefixes before tenantID are allowed for S3 bucket paths.
func ValidateTenantPath(raw, tenantID string) (string, error) {
	if strings.Contains(raw, "\\") {
		return "", fmt.Errorf("invalid media key")
	}
	clean := strings.TrimPrefix(path.Clean("/"+raw), "/")
	if clean == "." || clean == "" || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid media key")
	}
	if tenantID == "" {
		return clean, nil
	}
	// UploadMedia always introduces tenant_id as the first UUID-shaped path
	// component. Checking the first UUID (instead of merely searching for the
	// caller's UUID anywhere) prevents a path such as
	// other-tenant/file/<caller-tenant> from satisfying the ownership check.
	for _, component := range strings.Split(clean, "/") {
		if _, err := uuid.FromString(component); err == nil {
			if component == tenantID {
				return clean, nil
			}
			return "", fmt.Errorf("media key belongs to another tenant")
		}
	}
	return "", fmt.Errorf("media key does not belong to tenant")
}
