package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

type testOCISigner struct{}

func (testOCISigner) Sign(*http.Request) error { return nil }

func testOCIClients(t *testing.T, handler http.HandlerFunc) (core.ComputeClient, core.VirtualNetworkClient, core.BlockstorageClient, identity.IdentityClient) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	noRetry := common.NoRetryPolicy()
	base := common.BaseClient{Host: server.URL, BasePath: "20160918", HTTPClient: server.Client(), Signer: testOCISigner{}, UserAgent: "provider-test", Configuration: common.CustomClientConfiguration{RetryPolicy: &noRetry}}
	configureOCIClient(&base)
	return core.ComputeClient{BaseClient: base}, core.VirtualNetworkClient{BaseClient: base}, core.BlockstorageClient{BaseClient: base}, identity.IdentityClient{BaseClient: base}
}

func testOCIProfile() domain.AccessProfile {
	return domain.AccessProfile{Name: "test", Provider: "oracle", CompartmentID: "compartment", Region: "us-phoenix-1", SubnetID: "subnet", AvailabilityDomain: "AD-1", ImageID: "image", Shape: "VM.Standard.A1.Flex"}
}

func testComputeProvider(t *testing.T, handler http.HandlerFunc) *OCIProvider {
	t.Helper()
	compute, network, block, identityClient := testOCIClients(t, handler)
	return &OCIProvider{profile: testOCIProfile(), compute: compute, network: network, block: block, identity: identityClient, tenancyID: "tenancy", pollInterval: time.Millisecond}
}

func testNetworkProvider(t *testing.T, handler http.HandlerFunc) *OCINetworkProvider {
	t.Helper()
	_, network, _, identityClient := testOCIClients(t, handler)
	return &OCINetworkProvider{profile: testOCIProfile(), network: network, identity: identityClient, pollInterval: time.Millisecond}
}

func writeOCIJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("etag", "version-1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeOCINotFound(w http.ResponseWriter) {
	writeOCIJSON(w, http.StatusNotFound, map[string]string{"code": "NotAuthorizedOrNotFound", "message": "not found"})
}

func ownedTestInstance(state core.InstanceLifecycleStateEnum) core.Instance {
	return core.Instance{
		Id: common.String("instance"), CompartmentId: common.String("compartment"), DisplayName: common.String("test-instance"), AvailabilityDomain: common.String("AD-1"), Shape: common.String("VM.Standard.A1.Flex"), Region: common.String("us-phoenix-1"), LifecycleState: state,
		FreeformTags: map[string]string{managedTag: "true", accessTag: "test", operationTag: "test-token", "infinity-iac-name": "test-instance", "infinity-iac-subnet": "subnet", "infinity-iac-os": "ubuntu-24.04", "infinity-iac-port": "8080", "infinity-iac-storage": "50"},
		ShapeConfig:  &core.InstanceShapeConfig{Ocpus: common.Float32(2), MemoryInGBs: common.Float32(4)},
	}
}

func testOCIContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}
