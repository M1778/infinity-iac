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

func TestOCIProviderRequiresRealProfileFile(t *testing.T) {
	profile := domain.AccessProfile{
		Provider:           "oracle",
		CredentialSource:   "oci-profile:DEFAULT",
		ConfigPath:         t.TempDir() + "/missing-config",
		Region:             "us-phoenix-1",
		CompartmentID:      "ocid1.compartment.oc1.example",
		SubnetID:           "ocid1.subnet.oc1.example",
		AvailabilityDomain: "PHX-AD-1",
		ImageID:            "ocid1.image.oc1.example",
		Shape:              "VM.Standard.E4.Flex",
	}
	if _, err := NewOCIProvider(profile); err == nil {
		t.Fatal("expected missing OCI profile file to fail; provider must not fabricate credentials")
	}
}
