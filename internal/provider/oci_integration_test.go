package provider

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
)

// Run explicitly with INFINITY_IAC_OCI_INTEGRATION=1 and the documented
// read-only resource variables. It never creates, changes, or deletes OCI
// resources. Credentials are loaded from the local OCI CLI config.
func TestOCIProviderLiveReadOnlyAccess(t *testing.T) {
	if os.Getenv("INFINITY_IAC_OCI_INTEGRATION") != "1" {
		t.Skip("set INFINITY_IAC_OCI_INTEGRATION=1 to run the live read-only OCI check")
	}
	profile := domain.AccessProfile{
		Provider:           "oracle",
		CredentialSource:   "oci-profile:" + envOrFail(t, "INFINITY_IAC_OCI_PROFILE", "DEFAULT"),
		ConfigPath:         os.Getenv("INFINITY_IAC_OCI_CONFIG"),
		Region:             envOrFail(t, "INFINITY_IAC_OCI_REGION", ""),
		CompartmentID:      envOrFail(t, "INFINITY_IAC_OCI_COMPARTMENT", ""),
		SubnetID:           envOrFail(t, "INFINITY_IAC_OCI_SUBNET", ""),
		AvailabilityDomain: envOrFail(t, "INFINITY_IAC_OCI_AVAILABILITY_DOMAIN", ""),
		ImageID:            envOrFail(t, "INFINITY_IAC_OCI_IMAGE", ""),
		Shape:              envOrFail(t, "INFINITY_IAC_OCI_SHAPE", "VM.Standard.E4.Flex"),
		Name:               "integration",
	}
	backend, err := NewOCIProvider(profile)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := backend.ValidateAccess(ctx); err != nil {
		t.Fatal(err)
	}
}

func envOrFail(t *testing.T, name, fallback string) string {
	t.Helper()
	if value := os.Getenv(name); value != "" {
		return value
	}
	if fallback != "" {
		return fallback
	}
	t.Fatalf("%s is required", name)
	return ""
}
