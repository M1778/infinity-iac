package provider

import (
	"errors"
	"testing"

	"github.com/M1778/infinity-iac/internal/domain"
)

func TestOCIProviderFailsClosed(t *testing.T) {
	_, err := NewOCIProvider(domain.AccessProfile{Provider: "oracle", CredentialSource: "oci-profile:DEFAULT", Region: "us-phoenix-1"})
	if !errors.Is(err, ErrOCIConfiguration) {
		t.Fatalf("expected incomplete configuration error, got %v", err)
	}
}

func TestOCIProviderRequiresCredentialAndRegion(t *testing.T) {
	if _, err := NewOCIProvider(domain.AccessProfile{CredentialSource: "oci-profile:DEFAULT", Region: "local"}); err == nil {
		t.Fatal("expected missing provider to fail")
	}
	if _, err := NewOCIProvider(domain.AccessProfile{Provider: "oracle", Region: "local"}); err == nil {
		t.Fatal("expected missing credential source to fail")
	}
	if _, err := NewOCIProvider(domain.AccessProfile{Provider: "oracle", CredentialSource: "oci-profile:DEFAULT"}); err == nil {
		t.Fatal("expected missing region to fail")
	}
}
