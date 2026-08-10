package control

import (
	"context"
	"errors"
	"testing"
)

type fakeDNSResolver struct {
	txt   map[string][]string
	cname map[string]string
	err   error
}

func (f fakeDNSResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.txt[name], nil
}

func (f fakeDNSResolver) LookupCNAME(_ context.Context, name string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	value, ok := f.cname[name]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func TestValidTenantHostname(t *testing.T) {
	valid := []string{"mail.example.com", "news.customer.com.br", "a-b.example.org"}
	invalid := []string{"localhost", "mail.local", "*.example.com", "127.0.0.1", "bad host.example", "-bad.example.com", "example"}
	for _, value := range valid {
		if !validTenantHostname(value) {
			t.Errorf("expected valid hostname %q", value)
		}
	}
	for _, value := range invalid {
		if validTenantHostname(value) {
			t.Errorf("expected invalid hostname %q", value)
		}
	}
}

func TestDomainOwnershipLookup(t *testing.T) {
	txt := TenantDomain{Hostname: "mail.example.com", VerificationMethod: "txt", VerificationToken: "mailview-verify-token"}
	txt.VerificationName, txt.VerificationValue = domainVerificationRecord(txt)
	svc := &Service{resolver: fakeDNSResolver{txt: map[string][]string{txt.VerificationName: {"other", txt.VerificationValue}}}}
	if ok, reason := svc.lookupDomainOwnership(context.Background(), txt); !ok || reason != "" {
		t.Fatalf("TXT ownership = %t, %q", ok, reason)
	}

	cname := TenantDomain{Hostname: "news.example.com", VerificationMethod: "cname", VerificationToken: "mailview-verify-abc123"}
	cname.VerificationName, cname.VerificationValue = domainVerificationRecord(cname)
	svc.resolver = fakeDNSResolver{cname: map[string]string{cname.VerificationName: cname.VerificationValue + "."}}
	if ok, reason := svc.lookupDomainOwnership(context.Background(), cname); !ok || reason != "" {
		t.Fatalf("CNAME ownership = %t, %q", ok, reason)
	}
}

func TestNormalizeSlugRejectsReserved(t *testing.T) {
	if _, err := NormalizeSlug("admin"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reserved slug accepted: %v", err)
	}
	if got, err := NormalizeSlug("customer-one"); err != nil || got != "customer-one" {
		t.Fatalf("normal slug = %q, %v", got, err)
	}
}
