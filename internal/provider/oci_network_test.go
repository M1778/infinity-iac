package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

func testNetworkTags(kind string) map[string]string {
	return map[string]string{managedTag: "true", accessTag: "test", networkTag: "test-network", "infinity-iac-kind": kind, operationTag: "network-token"}
}

func testVcn() core.Vcn {
	return core.Vcn{Id: common.String("vcn"), CompartmentId: common.String("compartment"), DisplayName: common.String("test-network"), CidrBlock: common.String("10.88.0.0/16"), CidrBlocks: []string{"10.88.0.0/16"}, LifecycleState: core.VcnLifecycleStateAvailable, FreeformTags: testNetworkTags("vcn"), DefaultRouteTableId: common.String("route"), DefaultSecurityListId: common.String("security"), DefaultDhcpOptionsId: common.String("dhcp")}
}

func testSubnet() core.Subnet {
	return core.Subnet{Id: common.String("subnet"), CompartmentId: common.String("compartment"), VcnId: common.String("vcn"), CidrBlock: common.String("10.88.42.0/24"), LifecycleState: core.SubnetLifecycleStateAvailable, FreeformTags: testNetworkTags("subnet"), RouteTableId: common.String("route"), DhcpOptionsId: common.String("dhcp"), SecurityListIds: []string{"security"}}
}

func testGateway() core.InternetGateway {
	return core.InternetGateway{Id: common.String("gateway"), CompartmentId: common.String("compartment"), VcnId: common.String("vcn"), LifecycleState: core.InternetGatewayLifecycleStateAvailable, IsEnabled: common.Bool(true), FreeformTags: testNetworkTags("internet-gateway")}
}

func testRoute() core.RouteTable {
	return core.RouteTable{Id: common.String("route"), CompartmentId: common.String("compartment"), VcnId: common.String("vcn"), LifecycleState: core.RouteTableLifecycleStateAvailable, FreeformTags: testNetworkTags("route-table"), RouteRules: []core.RouteRule{{Destination: common.String("0.0.0.0/0"), DestinationType: core.RouteRuleDestinationTypeCidrBlock, NetworkEntityId: common.String("gateway")}}}
}

func testSecurity() core.SecurityList {
	return core.SecurityList{Id: common.String("security"), CompartmentId: common.String("compartment"), VcnId: common.String("vcn"), LifecycleState: core.SecurityListLifecycleStateAvailable, FreeformTags: testNetworkTags("security-list"), IngressSecurityRules: sshIngress("192.0.2.10/32"), EgressSecurityRules: []core.EgressSecurityRule{{Protocol: common.String("all"), Destination: common.String("0.0.0.0/0"), DestinationType: core.EgressSecurityRuleDestinationTypeCidrBlock}}}
}

func testDhcp() core.DhcpOptions {
	return core.DhcpOptions{Id: common.String("dhcp"), CompartmentId: common.String("compartment"), VcnId: common.String("vcn"), LifecycleState: core.DhcpOptionsLifecycleStateAvailable, FreeformTags: testNetworkTags("dhcp-options")}
}

func TestOCINetworkOnlyValidationIsReadOnly(t *testing.T) {
	var requests atomic.Int32
	p := testNetworkProvider(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.URL.Path != "/20160918/compartments/compartment" {
			t.Errorf("unexpected network validation request: %s %s", r.Method, r.URL.String())
		}
		writeOCIJSON(w, 200, identity.Compartment{Id: common.String("compartment"), LifecycleState: identity.CompartmentLifecycleStateActive})
	})
	p.profile.SubnetID, p.profile.ImageID, p.profile.Shape, p.profile.AvailabilityDomain = "", "", "", ""
	if err := p.ValidateAccess(testOCIContext(t)); err != nil || requests.Load() != 1 {
		t.Fatalf("network-only preflight failed: %v", err)
	}
}

func TestOCINetworkMutationRejectsPersonalVCN(t *testing.T) {
	var mutations atomic.Int32
	p := testNetworkProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			mutations.Add(1)
		}
		vcn := testVcn()
		vcn.FreeformTags = nil
		writeOCIJSON(w, 200, vcn)
	})
	ctx := testOCIContext(t)
	if _, err := p.GetNetwork(ctx, "vcn"); !errors.Is(err, ErrOCIOwnership) {
		t.Errorf("personal VCN was adopted: %v", err)
	}
	if _, err := p.UpdateNetwork(ctx, "vcn", "192.0.2.11/32"); !errors.Is(err, ErrOCIOwnership) {
		t.Errorf("personal VCN was updated: %v", err)
	}
	if err := p.DeleteNetwork(ctx, "vcn"); !errors.Is(err, ErrOCIOwnership) {
		t.Errorf("personal VCN was deleted: %v", err)
	}
	if mutations.Load() != 0 {
		t.Fatal("personal VCN mutation occurred")
	}
}

func TestOCINetworkCreateConfiguresOwnedSafeDefaults(t *testing.T) {
	vcn := testVcn()
	route := testRoute()
	security := testSecurity()
	dhcp := testDhcp()
	var steps []string
	p := testNetworkProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			steps = append(steps, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/20160918/vcns":
			writeOCIJSON(w, 200, []core.Vcn{})
		case r.Method == "POST" && r.URL.Path == "/20160918/vcns":
			var details core.CreateVcnDetails
			if err := json.NewDecoder(r.Body).Decode(&details); err != nil {
				t.Error(err)
			}
			if len(details.CidrBlocks) != 1 || details.CidrBlocks[0] != "10.88.0.0/16" || value(details.DisplayName) != "test-network" || details.FreeformTags[accessTag] != "test" || details.FreeformTags[networkTag] != "test-network" || details.FreeformTags[operationTag] != "stable-network" || r.Header.Get("opc-retry-token") != "stable-network" || !strings.HasPrefix(value(details.DnsLabel), "iac") {
				t.Error("VCN request lost ownership, retry token, DNS or CIDR")
			}
			vcn.FreeformTags = details.FreeformTags
			writeOCIJSON(w, 200, vcn)
		case r.Method == "GET" && r.URL.Path == "/20160918/vcns/vcn":
			writeOCIJSON(w, 200, vcn)
		case r.Method == "GET" && r.URL.Path == "/20160918/routeTables/route":
			writeOCIJSON(w, 200, route)
		case r.Method == "PUT" && r.URL.Path == "/20160918/routeTables/route":
			if r.Header.Get("if-match") != "version-1" {
				t.Error("route update has no ETag")
			}
			var details core.UpdateRouteTableDetails
			if err := json.NewDecoder(r.Body).Decode(&details); err != nil {
				t.Error(err)
			}
			if details.FreeformTags != nil {
				route.FreeformTags = details.FreeformTags
			}
			if len(details.RouteRules) == 1 && (value(details.RouteRules[0].Destination) != "0.0.0.0/0" || value(details.RouteRules[0].NetworkEntityId) != "gateway") {
				t.Error("outbound route did not target the owned gateway")
			}
			route.RouteRules = details.RouteRules
			writeOCIJSON(w, 200, route)
		case r.Method == "GET" && r.URL.Path == "/20160918/securityLists/security":
			writeOCIJSON(w, 200, security)
		case r.Method == "PUT" && r.URL.Path == "/20160918/securityLists/security":
			if r.Header.Get("if-match") != "version-1" {
				t.Error("security update has no ETag")
			}
			var details core.UpdateSecurityListDetails
			if err := json.NewDecoder(r.Body).Decode(&details); err != nil {
				t.Error(err)
			}
			if len(details.IngressSecurityRules) != 2 || value(details.IngressSecurityRules[0].Source) != "192.0.2.10/32" || value(details.IngressSecurityRules[0].Protocol) != "6" || *details.IngressSecurityRules[0].TcpOptions.DestinationPortRange.Min != 22 || *details.IngressSecurityRules[0].TcpOptions.DestinationPortRange.Max != 22 || *details.IngressSecurityRules[1].IcmpOptions.Type != 3 || *details.IngressSecurityRules[1].IcmpOptions.Code != 4 || len(details.EgressSecurityRules) != 1 || value(details.EgressSecurityRules[0].Protocol) != "all" {
				t.Error("unexpected public or missing SSH/MTU security rule")
			}
			security.FreeformTags = details.FreeformTags
			security.IngressSecurityRules = details.IngressSecurityRules
			security.EgressSecurityRules = details.EgressSecurityRules
			writeOCIJSON(w, 200, security)
		case r.Method == "GET" && r.URL.Path == "/20160918/dhcps/dhcp":
			writeOCIJSON(w, 200, dhcp)
		case r.Method == "PUT" && r.URL.Path == "/20160918/dhcps/dhcp":
			if r.Header.Get("if-match") != "version-1" {
				t.Error("DHCP update has no ETag")
			}
			var details core.UpdateDhcpDetails
			if err := json.NewDecoder(r.Body).Decode(&details); err != nil {
				t.Error(err)
			}
			dhcp.FreeformTags = details.FreeformTags
			writeOCIJSON(w, 200, dhcp)
		case r.Method == "POST" && r.URL.Path == "/20160918/internetGateways":
			var details core.CreateInternetGatewayDetails
			if err := json.NewDecoder(r.Body).Decode(&details); err != nil {
				t.Error(err)
			}
			if value(details.VcnId) != "vcn" || details.IsEnabled == nil || !*details.IsEnabled || details.FreeformTags[accessTag] != "test" || details.FreeformTags[networkTag] != "test-network" || r.Header.Get("opc-retry-token") != childOperationToken("stable-network", "gateway") {
				t.Error("gateway request lost identity or enabled state")
			}
			writeOCIJSON(w, 200, testGateway())
		case r.Method == "GET" && r.URL.Path == "/20160918/internetGateways/gateway":
			writeOCIJSON(w, 200, testGateway())
		case r.Method == "POST" && r.URL.Path == "/20160918/subnets":
			var details core.CreateSubnetDetails
			if err := json.NewDecoder(r.Body).Decode(&details); err != nil {
				t.Error(err)
			}
			if value(details.VcnId) != "vcn" || value(details.CidrBlock) != "10.88.42.0/24" || value(details.RouteTableId) != "route" || len(details.SecurityListIds) != 1 || details.SecurityListIds[0] != "security" || value(details.DhcpOptionsId) != "dhcp" || details.AvailabilityDomain != nil || r.Header.Get("opc-retry-token") != childOperationToken("stable-network", "subnet") {
				t.Error("subnet request not associated with exact regional network")
			}
			writeOCIJSON(w, 200, testSubnet())
		case r.Method == "GET" && r.URL.Path == "/20160918/subnets/subnet":
			writeOCIJSON(w, 200, testSubnet())
		default:
			t.Errorf("unexpected creation request: %s %s", r.Method, r.URL.String())
			writeOCINotFound(w)
		}
	})
	item, err := p.CreateNetwork(testOCIContext(t), NetworkCreateRequest{Spec: domain.NetworkSpec{Name: "test-network", CIDR: "10.88.0.0/16", SubnetCIDR: "10.88.42.0/24", SSHSource: "192.0.2.10/32"}, IdempotencyKey: "stable-network"})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "vcn" || item.SubnetID != "subnet" || item.InternetGatewayID != "gateway" || item.Status != "available" || item.SSHSource != "192.0.2.10/32" {
		t.Fatalf("incomplete network result: %+v", item)
	}
	if len(steps) != 7 || steps[0] != "POST /20160918/vcns" || steps[4] != "POST /20160918/internetGateways" || steps[6] != "POST /20160918/subnets" {
		t.Fatalf("unexpected network create order: %+v", steps)
	}
}

func networkDeleteHandler(t *testing.T, steps *[]string, foreignSubnet, occupied bool) http.HandlerFunc {
	t.Helper()
	var subnetDeleted, gatewayDeleted, vcnDeleted bool
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			*steps = append(*steps, r.Method+" "+r.URL.Path)
			if r.Header.Get("if-match") != "version-1" {
				t.Error("network mutation missing ETag")
			}
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/20160918/vcns/vcn":
			if vcnDeleted {
				writeOCINotFound(w)
			} else {
				writeOCIJSON(w, 200, testVcn())
			}
		case r.Method == "GET" && r.URL.Path == "/20160918/subnets":
			subnet := testSubnet()
			if foreignSubnet {
				subnet.FreeformTags = nil
			}
			writeOCIJSON(w, 200, []core.Subnet{subnet})
		case r.Method == "GET" && r.URL.Path == "/20160918/internetGateways":
			writeOCIJSON(w, 200, []core.InternetGateway{testGateway()})
		case r.Method == "GET" && r.URL.Path == "/20160918/routeTables":
			writeOCIJSON(w, 200, []core.RouteTable{testRoute()})
		case r.Method == "GET" && r.URL.Path == "/20160918/securityLists":
			writeOCIJSON(w, 200, []core.SecurityList{testSecurity()})
		case r.Method == "GET" && r.URL.Path == "/20160918/dhcps":
			writeOCIJSON(w, 200, []core.DhcpOptions{testDhcp()})
		case r.Method == "GET" && (r.URL.Path == "/20160918/natGateways" || r.URL.Path == "/20160918/serviceGateways" || r.URL.Path == "/20160918/localPeeringGateways" || r.URL.Path == "/20160918/networkSecurityGroups" || r.URL.Path == "/20160918/drgAttachments" || r.URL.Path == "/20160918/vlans"):
			writeOCIJSON(w, 200, []any{})
		case r.Method == "GET" && r.URL.Path == "/20160918/privateIps":
			if occupied {
				writeOCIJSON(w, 200, []core.PrivateIp{{Id: common.String("personal-ip"), SubnetId: common.String("subnet"), VnicId: common.String("personal-vnic")}})
			} else {
				writeOCIJSON(w, 200, []core.PrivateIp{})
			}
		case r.Method == "GET" && r.URL.Path == "/20160918/subnets/subnet":
			if subnetDeleted {
				writeOCINotFound(w)
			} else {
				writeOCIJSON(w, 200, testSubnet())
			}
		case r.Method == "DELETE" && r.URL.Path == "/20160918/subnets/subnet":
			subnetDeleted = true
			w.WriteHeader(204)
		case r.Method == "GET" && r.URL.Path == "/20160918/routeTables/route":
			writeOCIJSON(w, 200, testRoute())
		case r.Method == "PUT" && r.URL.Path == "/20160918/routeTables/route":
			var details core.UpdateRouteTableDetails
			if err := json.NewDecoder(r.Body).Decode(&details); err != nil || details.RouteRules == nil || len(details.RouteRules) != 0 {
				t.Error("route rules were not explicitly cleared")
			}
			writeOCIJSON(w, 200, testRoute())
		case r.Method == "GET" && r.URL.Path == "/20160918/internetGateways/gateway":
			if gatewayDeleted {
				writeOCINotFound(w)
			} else {
				writeOCIJSON(w, 200, testGateway())
			}
		case r.Method == "DELETE" && r.URL.Path == "/20160918/internetGateways/gateway":
			gatewayDeleted = true
			w.WriteHeader(204)
		case r.Method == "DELETE" && r.URL.Path == "/20160918/vcns/vcn":
			vcnDeleted = true
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected delete request: %s %s", r.Method, r.URL.String())
			writeOCINotFound(w)
		}
	}
}

func TestOCINetworkDeleteOrdersDependenciesAndConfirmsAbsence(t *testing.T) {
	var steps []string
	p := testNetworkProvider(t, networkDeleteHandler(t, &steps, false, false))
	if err := p.DeleteNetwork(testOCIContext(t), "vcn"); err != nil {
		t.Fatal(err)
	}
	want := []string{"DELETE /20160918/subnets/subnet", "PUT /20160918/routeTables/route", "DELETE /20160918/internetGateways/gateway", "DELETE /20160918/vcns/vcn"}
	if strings.Join(steps, ";") != strings.Join(want, ";") {
		t.Fatalf("unsafe delete order: %+v", steps)
	}
}

func TestOCINetworkDeleteRejectsForeignAndOccupiedChildrenBeforeMutating(t *testing.T) {
	for _, foreign := range []bool{true, false} {
		var steps []string
		p := testNetworkProvider(t, networkDeleteHandler(t, &steps, foreign, !foreign))
		if err := p.DeleteNetwork(testOCIContext(t), "vcn"); err == nil || len(steps) != 0 {
			t.Fatalf("foreign dependency was touched: %+v %v", steps, err)
		}
	}
}

func TestOCINetworkCreateFailureRollsBackOwnedResources(t *testing.T) {
	var steps []string
	deleteHandler := networkDeleteHandler(t, &steps, false, false)
	var rollback bool
	vcn := testVcn()
	p := testNetworkProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if rollback {
			deleteHandler(w, r)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/20160918/vcns":
			writeOCIJSON(w, 200, []core.Vcn{})
		case r.Method == "POST" && r.URL.Path == "/20160918/vcns":
			writeOCIJSON(w, 200, vcn)
		case r.Method == "GET" && r.URL.Path == "/20160918/vcns/vcn":
			writeOCIJSON(w, 200, vcn)
		case r.Method == "GET" && r.URL.Path == "/20160918/routeTables/route":
			writeOCIJSON(w, 200, testRoute())
		case r.Method == "PUT" && r.URL.Path == "/20160918/routeTables/route":
			rollback = true
			writeOCIJSON(w, 400, map[string]string{"code": "InvalidParameter", "message": "injected create failure"})
		default:
			t.Errorf("unexpected rollback setup request: %s %s", r.Method, r.URL.String())
			writeOCINotFound(w)
		}
	})
	item, err := p.CreateNetwork(testOCIContext(t), NetworkCreateRequest{Spec: domain.NetworkSpec{Name: "test-network", CIDR: "10.88.0.0/16", SubnetCIDR: "10.88.42.0/24", SSHSource: "192.0.2.10/32"}, IdempotencyKey: "network-token"})
	if err == nil || item.ID != "vcn" || item.Status != domain.StatusDeleted || len(steps) != 4 {
		t.Fatalf("partial creation was not returned and rolled back: %+v %+v %v", item, steps, err)
	}
}

func TestOCINetworkWaitHonorsCancellationAndUnknown404(t *testing.T) {
	p := testNetworkProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/absent") {
			writeOCINotFound(w)
			return
		}
		vcn := testVcn()
		vcn.LifecycleState = core.VcnLifecycleStateProvisioning
		writeOCIJSON(w, 200, vcn)
	})
	if _, err := p.WaitNetwork(testOCIContext(t), "absent", domain.StatusDeleted); err == nil {
		t.Fatal("unobserved 404 was treated as an owned network deletion")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.WaitNetwork(ctx, "vcn", "available"); !errors.Is(err, context.Canceled) {
		t.Fatalf("network wait ignored cancellation: %v", err)
	}
}

func TestOCINetworkUpdateOnlyAllowsNarrowSSHAndUsesETag(t *testing.T) {
	var updates atomic.Int32
	security := testSecurity()
	p := testNetworkProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/20160918/vcns/vcn":
			writeOCIJSON(w, 200, testVcn())
		case r.URL.Path == "/20160918/subnets":
			writeOCIJSON(w, 200, []core.Subnet{testSubnet()})
		case r.URL.Path == "/20160918/internetGateways":
			writeOCIJSON(w, 200, []core.InternetGateway{testGateway()})
		case r.Method == "GET" && r.URL.Path == "/20160918/securityLists/security":
			writeOCIJSON(w, 200, security)
		case r.Method == "PUT" && r.URL.Path == "/20160918/securityLists/security":
			updates.Add(1)
			if r.Header.Get("if-match") != "version-1" {
				t.Error("update did not use current ETag")
			}
			var details core.UpdateSecurityListDetails
			if err := json.NewDecoder(r.Body).Decode(&details); err != nil {
				t.Error(err)
			}
			if details.EgressSecurityRules != nil || details.FreeformTags != nil || len(details.IngressSecurityRules) != 2 {
				t.Error("SSH update changed unrelated state")
			}
			security.IngressSecurityRules = details.IngressSecurityRules
			writeOCIJSON(w, 200, security)
		default:
			t.Errorf("unexpected update request: %s %s", r.Method, r.URL.String())
			writeOCINotFound(w)
		}
	})
	if _, err := p.UpdateNetwork(testOCIContext(t), "vcn", "0.0.0.0/0"); err == nil || updates.Load() != 0 {
		t.Fatal("public SSH was allowed")
	}
	item, err := p.UpdateNetwork(testOCIContext(t), "vcn", "192.0.2.11/32")
	if err != nil || updates.Load() != 1 || item.SSHSource != "192.0.2.11/32" || item.SubnetID != "subnet" {
		t.Fatalf("safe SSH update failed: %+v %v", item, err)
	}
}
