package provider

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
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

func TestOCILiveReadOnlyCredentialsAndNetworkInventory(t *testing.T) {
	if os.Getenv("INFINITY_IAC_OCI_AUTH_TEST") != "1" {
		t.Skip("set INFINITY_IAC_OCI_AUTH_TEST=1 for read-only credentials and network inventory")
	}
	metadata, err := DetectOCIConfiguration(os.Getenv("INFINITY_IAC_OCI_CONFIG"), envOrFail(t, "INFINITY_IAC_OCI_PROFILE", "DEFAULT"))
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := common.ConfigurationProviderFromFileWithProfile(metadata.Path, envOrFail(t, "INFINITY_IAC_OCI_PROFILE", "DEFAULT"), "")
	if err != nil {
		t.Fatal(err)
	}
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}
	networkClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}
	computeClient, err := core.NewComputeClientWithConfigurationProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tenancy := common.String(metadata.TenancyID)
	subscriptions, err := identityClient.ListRegionSubscriptions(ctx, identity.ListRegionSubscriptionsRequest{TenancyId: tenancy})
	if err != nil {
		t.Fatal(err)
	}
	regionReady := false
	for _, region := range subscriptions.Items {
		if region.RegionName != nil && *region.RegionName == metadata.Region && region.Status == identity.RegionSubscriptionStatusReady {
			regionReady = true
		}
	}
	if !regionReady {
		t.Fatal("configured region subscription is not ready")
	}
	domains, err := identityClient.ListAvailabilityDomains(ctx, identity.ListAvailabilityDomainsRequest{CompartmentId: tenancy})
	if err != nil {
		t.Fatal(err)
	}
	if len(domains.Items) == 0 {
		t.Fatal("no availability domains returned")
	}
	compartment := metadata.TenancyID
	if value := os.Getenv("INFINITY_IAC_OCI_COMPARTMENT"); value != "" {
		compartment = value
	}
	vcns, err := networkClient.ListVcns(ctx, core.ListVcnsRequest{CompartmentId: common.String(compartment)})
	if err != nil {
		t.Fatal(err)
	}
	subnets, err := networkClient.ListSubnets(ctx, core.ListSubnetsRequest{CompartmentId: common.String(compartment)})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := computeClient.ListInstances(ctx, core.ListInstancesRequest{CompartmentId: common.String(compartment), Limit: common.Int(100)})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("region %s authenticated; %d availability domains; first-page inventory: %d VCNs, %d subnets, %d instances", metadata.Region, len(domains.Items), len(vcns.Items), len(subnets.Items), len(instances.Items))
}
