package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

type OCINetworkProvider struct {
	profile      domain.AccessProfile
	network      core.VirtualNetworkClient
	identity     identity.IdentityClient
	pollInterval time.Duration
}

func NewOCINetworkProvider(profile domain.AccessProfile) (*OCINetworkProvider, error) {
	config, _, err := loadOCIConfiguration(profile)
	if err != nil {
		return nil, err
	}
	network, err := core.NewVirtualNetworkClientWithConfigurationProvider(config)
	if err != nil {
		return nil, fmt.Errorf("create OCI network client: %w", err)
	}
	network.SetRegion(profile.Region)
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(config)
	if err != nil {
		return nil, fmt.Errorf("create OCI identity client: %w", err)
	}
	identityClient.SetRegion(profile.Region)
	configureOCIClient(&network.BaseClient)
	configureOCIClient(&identityClient.BaseClient)
	return &OCINetworkProvider{profile: profile, network: network, identity: identityClient}, nil
}

func (p *OCINetworkProvider) Profile() domain.AccessProfile { return p.profile }
func (p *OCINetworkProvider) Name() string                  { return "oracle" }

func (p *OCINetworkProvider) ValidateAccess(ctx context.Context) error {
	return validateOCICompartment(ctx, p.identity, p.profile.CompartmentID)
}

func (p *OCINetworkProvider) networkTags(name, kind, token string) map[string]string {
	return map[string]string{managedTag: "true", accessTag: p.profile.Name, networkTag: name, "infinity-iac-kind": kind, operationTag: token}
}

func (p *OCINetworkProvider) ownChild(compartmentID, vcnID *string, tags map[string]string, vcn core.Vcn) error {
	if err := ownOCIResource(p.profile, compartmentID, tags); err != nil {
		return err
	}
	if value(vcnID) != value(vcn.Id) || tags[networkTag] != vcn.FreeformTags[networkTag] {
		return ErrOCIOwnership
	}
	return nil
}

func (p *OCINetworkProvider) getOwnedVcn(ctx context.Context, id string) (core.GetVcnResponse, error) {
	if err := ctx.Err(); err != nil {
		return core.GetVcnResponse{}, err
	}
	if strings.TrimSpace(id) == "" {
		return core.GetVcnResponse{}, errors.New("network ID is required")
	}
	resp, err := p.network.GetVcn(ociRequestContext(ctx), core.GetVcnRequest{VcnId: common.String(id)})
	if err != nil {
		return resp, fmt.Errorf("get OCI VCN: %w", err)
	}
	if value(resp.Id) != id {
		return resp, errors.New("OCI VCN response did not match the requested ID")
	}
	if err := ownOCIResource(p.profile, resp.CompartmentId, resp.FreeformTags); err != nil {
		return resp, err
	}
	if resp.FreeformTags[networkTag] == "" || resp.FreeformTags[networkTag] != value(resp.DisplayName) {
		return resp, ErrOCIOwnership
	}
	return resp, nil
}

func (p *OCINetworkProvider) fromVcn(vcn core.Vcn) domain.Network {
	return domain.Network{ID: value(vcn.Id), Name: value(vcn.DisplayName), Provider: "oracle", Access: p.profile.Name, Region: p.profile.Region, CIDR: value(vcn.CidrBlock), RouteTableID: value(vcn.DefaultRouteTableId), SecurityListID: value(vcn.DefaultSecurityListId), Status: strings.ToLower(string(vcn.LifecycleState))}
}

func (p *OCINetworkProvider) ListNetworks(ctx context.Context) ([]domain.Network, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items := make([]domain.Network, 0)
	var page *string
	for {
		resp, err := p.network.ListVcns(ociRequestContext(ctx), core.ListVcnsRequest{CompartmentId: common.String(p.profile.CompartmentID), Page: page})
		if err != nil {
			return nil, fmt.Errorf("list OCI VCNs: %w", err)
		}
		for _, vcn := range resp.Items {
			if ownOCIResource(p.profile, vcn.CompartmentId, vcn.FreeformTags) != nil || value(vcn.Id) == "" || vcn.LifecycleState == core.VcnLifecycleStateTerminated {
				continue
			}
			item, err := p.GetNetwork(ctx, *vcn.Id)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			return items, nil
		}
	}
}

func (p *OCINetworkProvider) GetNetwork(ctx context.Context, id string) (domain.Network, error) {
	vcn, err := p.getOwnedVcn(ctx, id)
	if err != nil {
		return domain.Network{}, err
	}
	item := p.fromVcn(vcn.Vcn)
	if vcn.LifecycleState == core.VcnLifecycleStateTerminated {
		item.Status = domain.StatusDeleted
		return item, nil
	}
	subnets, gateways, err := p.networkChildren(ctx, vcn.Vcn)
	if err != nil {
		return item, err
	}
	if len(subnets) > 0 {
		item.SubnetID, item.SubnetCIDR = value(subnets[0].Id), value(subnets[0].CidrBlock)
	}
	if len(gateways) > 0 {
		item.InternetGatewayID = value(gateways[0].Id)
	}
	security, err := p.network.GetSecurityList(ociRequestContext(ctx), core.GetSecurityListRequest{SecurityListId: vcn.DefaultSecurityListId})
	if err != nil {
		return item, fmt.Errorf("inspect OCI network security list: %w", err)
	}
	if value(security.Id) != item.SecurityListID || p.ownChild(security.CompartmentId, security.VcnId, security.FreeformTags, vcn.Vcn) != nil {
		return item, ErrOCIOwnership
	}
	for _, rule := range security.IngressSecurityRules {
		if value(rule.Protocol) == "6" && rule.TcpOptions != nil && rule.TcpOptions.DestinationPortRange != nil && rule.TcpOptions.DestinationPortRange.Min != nil && *rule.TcpOptions.DestinationPortRange.Min == 22 && rule.TcpOptions.DestinationPortRange.Max != nil && *rule.TcpOptions.DestinationPortRange.Max == 22 {
			item.SSHSource = value(rule.Source)
		}
	}
	return item, nil
}

func (p *OCINetworkProvider) networkChildren(ctx context.Context, vcn core.Vcn) ([]core.Subnet, []core.InternetGateway, error) {
	var subnets []core.Subnet
	var gateways []core.InternetGateway
	var page *string
	for {
		resp, err := p.network.ListSubnets(ociRequestContext(ctx), core.ListSubnetsRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Page: page})
		if err != nil {
			return nil, nil, fmt.Errorf("list OCI network subnets: %w", err)
		}
		for _, subnet := range resp.Items {
			if subnet.LifecycleState == core.SubnetLifecycleStateTerminated {
				continue
			}
			if value(subnet.Id) == "" || p.ownChild(subnet.CompartmentId, subnet.VcnId, subnet.FreeformTags, vcn) != nil || subnet.FreeformTags["infinity-iac-kind"] != "subnet" {
				return nil, nil, errors.New("OCI network contains a foreign subnet; refusing network mutation")
			}
			if value(subnet.RouteTableId) != value(vcn.DefaultRouteTableId) || len(subnet.SecurityListIds) != 1 || subnet.SecurityListIds[0] != value(vcn.DefaultSecurityListId) || value(subnet.DhcpOptionsId) != value(vcn.DefaultDhcpOptionsId) {
				return nil, nil, errors.New("OCI subnet is associated with unexpected network resources")
			}
			subnets = append(subnets, subnet)
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	page = nil
	for {
		resp, err := p.network.ListInternetGateways(ociRequestContext(ctx), core.ListInternetGatewaysRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Page: page})
		if err != nil {
			return nil, nil, fmt.Errorf("list OCI network gateways: %w", err)
		}
		for _, gateway := range resp.Items {
			if gateway.LifecycleState == core.InternetGatewayLifecycleStateTerminated {
				continue
			}
			if value(gateway.Id) == "" || p.ownChild(gateway.CompartmentId, gateway.VcnId, gateway.FreeformTags, vcn) != nil || gateway.FreeformTags["infinity-iac-kind"] != "internet-gateway" {
				return nil, nil, errors.New("OCI network contains a foreign gateway; refusing network mutation")
			}
			gateways = append(gateways, gateway)
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	if len(subnets) > 1 || len(gateways) > 1 {
		return nil, nil, errors.New("OCI network contains unexpected additional managed resources")
	}
	return subnets, gateways, nil
}

func (p *OCINetworkProvider) CreateNetwork(ctx context.Context, req NetworkCreateRequest) (result domain.Network, err error) {
	req.Spec = req.Spec.ResolveDefaults(p.profile.Name, p.profile.Region)
	if err := req.Spec.Validate(); err != nil {
		return result, err
	}
	if req.Spec.Access != p.profile.Name || req.Spec.Region != p.profile.Region {
		return result, errors.New("network access or region does not match the configured OCI profile")
	}
	token, err := operationToken(req.IdempotencyKey)
	if err != nil {
		return result, err
	}
	ctx, cancel := boundedOCIContext(ctx)
	defer cancel()
	var existing *core.Vcn
	var page *string
	for {
		resp, listErr := p.network.ListVcns(ociRequestContext(ctx), core.ListVcnsRequest{CompartmentId: common.String(p.profile.CompartmentID), DisplayName: common.String(req.Spec.Name), Page: page})
		if listErr != nil {
			return result, fmt.Errorf("inspect OCI network launch identity: %w", listErr)
		}
		for _, vcn := range resp.Items {
			if value(vcn.DisplayName) != req.Spec.Name || vcn.LifecycleState == core.VcnLifecycleStateTerminated {
				continue
			}
			if ownOCIResource(p.profile, vcn.CompartmentId, vcn.FreeformTags) != nil || vcn.FreeformTags[networkTag] != req.Spec.Name || vcn.FreeformTags[operationTag] != token || value(vcn.CidrBlock) != req.Spec.CIDR || vcn.FreeformTags["infinity-iac-subnet-cidr"] != req.Spec.SubnetCIDR || vcn.FreeformTags["infinity-iac-ssh-source"] != req.Spec.SSHSource || existing != nil {
				return result, fmt.Errorf("network name %q already exists outside this exact creation request", req.Spec.Name)
			}
			copy := vcn
			existing = &copy
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	if existing != nil {
		item, err := p.GetNetwork(ctx, value(existing.Id))
		if err == nil && (item.SubnetID == "" || item.InternetGatewayID == "" || item.SubnetCIDR != req.Spec.SubnetCIDR || item.SSHSource != req.Spec.SSHSource || item.Status != "available") {
			err = errors.New("the exact OCI network request is incomplete or changed; inspect and clean it before creating another")
		}
		return item, err
	}
	tags := p.networkTags(req.Spec.Name, "vcn", token)
	tags["infinity-iac-subnet-cidr"], tags["infinity-iac-ssh-source"] = req.Spec.SubnetCIDR, req.Spec.SSHSource
	created, err := p.network.CreateVcn(ociRequestContext(ctx), core.CreateVcnRequest{CreateVcnDetails: core.CreateVcnDetails{CompartmentId: common.String(p.profile.CompartmentID), DisplayName: common.String(req.Spec.Name), CidrBlocks: []string{req.Spec.CIDR}, DnsLabel: common.String("iac" + childOperationToken(token, "dns")[:12]), FreeformTags: tags}, OpcRetryToken: common.String(token)})
	if err != nil {
		return result, fmt.Errorf("create OCI VCN: %w", err)
	}
	result = p.fromVcn(created.Vcn)
	if result.ID == "" {
		return result, errors.New("OCI VCN creation returned no ID")
	}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if cleanupErr := p.deleteNetwork(cleanup, result.ID, true); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("partial OCI network cleanup failed: %w", cleanupErr))
			} else {
				result.Status = domain.StatusDeleted
			}
		}
	}()
	if err = p.waitVcn(ctx, result.ID, core.VcnLifecycleStateAvailable); err != nil {
		return result, err
	}
	vcn, err := p.getOwnedVcn(ctx, result.ID)
	if err != nil {
		return result, err
	}
	result = p.fromVcn(vcn.Vcn)
	if err = p.initializeDefaults(ctx, vcn.Vcn, req.Spec.SSHSource, token); err != nil {
		return result, err
	}
	gateway, err := p.network.CreateInternetGateway(ociRequestContext(ctx), core.CreateInternetGatewayRequest{CreateInternetGatewayDetails: core.CreateInternetGatewayDetails{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, DisplayName: common.String(req.Spec.Name + "-gateway"), IsEnabled: common.Bool(true), FreeformTags: p.networkTags(req.Spec.Name, "internet-gateway", childOperationToken(token, "gateway"))}, OpcRetryToken: common.String(childOperationToken(token, "gateway"))})
	if err != nil {
		return result, fmt.Errorf("create OCI internet gateway: %w", err)
	}
	result.InternetGatewayID = value(gateway.Id)
	if result.InternetGatewayID == "" {
		return result, errors.New("OCI gateway creation returned no ID")
	}
	if err = p.waitGateway(ctx, result.InternetGatewayID, core.InternetGatewayLifecycleStateAvailable); err != nil {
		return result, err
	}
	route, err := p.network.GetRouteTable(ociRequestContext(ctx), core.GetRouteTableRequest{RtId: vcn.DefaultRouteTableId})
	if err != nil {
		return result, fmt.Errorf("inspect OCI default route table: %w", err)
	}
	if p.ownChild(route.CompartmentId, route.VcnId, route.FreeformTags, vcn.Vcn) != nil {
		return result, ErrOCIOwnership
	}
	match, err := etag(route.Etag)
	if err != nil {
		return result, err
	}
	_, err = p.network.UpdateRouteTable(ociRequestContext(ctx), core.UpdateRouteTableRequest{RtId: route.Id, IfMatch: match, UpdateRouteTableDetails: core.UpdateRouteTableDetails{RouteRules: []core.RouteRule{{Destination: common.String("0.0.0.0/0"), DestinationType: core.RouteRuleDestinationTypeCidrBlock, NetworkEntityId: gateway.Id}}}})
	if err != nil {
		return result, fmt.Errorf("configure OCI outbound route: %w", err)
	}
	subnet, err := p.network.CreateSubnet(ociRequestContext(ctx), core.CreateSubnetRequest{CreateSubnetDetails: core.CreateSubnetDetails{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, CidrBlock: common.String(req.Spec.SubnetCIDR), DisplayName: common.String(req.Spec.Name + "-subnet"), DnsLabel: common.String("apps"), RouteTableId: vcn.DefaultRouteTableId, SecurityListIds: []string{value(vcn.DefaultSecurityListId)}, DhcpOptionsId: vcn.DefaultDhcpOptionsId, ProhibitPublicIpOnVnic: common.Bool(false), FreeformTags: p.networkTags(req.Spec.Name, "subnet", childOperationToken(token, "subnet"))}, OpcRetryToken: common.String(childOperationToken(token, "subnet"))})
	if err != nil {
		return result, fmt.Errorf("create OCI subnet: %w", err)
	}
	result.SubnetID, result.SubnetCIDR, result.SSHSource = value(subnet.Id), req.Spec.SubnetCIDR, req.Spec.SSHSource
	if result.SubnetID == "" {
		return result, errors.New("OCI subnet creation returned no ID")
	}
	if err = p.waitSubnet(ctx, result.SubnetID, core.SubnetLifecycleStateAvailable); err != nil {
		return result, err
	}
	result.Status = "available"
	return result, nil
}

func sshIngress(source string) []core.IngressSecurityRule {
	return []core.IngressSecurityRule{
		{Protocol: common.String("6"), Source: common.String(source), SourceType: core.IngressSecurityRuleSourceTypeCidrBlock, TcpOptions: &core.TcpOptions{DestinationPortRange: &core.PortRange{Min: common.Int(22), Max: common.Int(22)}}, Description: common.String("SSH from the configured administrator network")},
		{Protocol: common.String("1"), Source: common.String("0.0.0.0/0"), SourceType: core.IngressSecurityRuleSourceTypeCidrBlock, IcmpOptions: &core.IcmpOptions{Type: common.Int(3), Code: common.Int(4)}, Description: common.String("Path MTU discovery")},
	}
}

func (p *OCINetworkProvider) defaultTags(vcn core.Vcn, compartmentID, vcnID *string, tags map[string]string, kind, token string) (map[string]string, error) {
	if value(compartmentID) != p.profile.CompartmentID || value(vcnID) != value(vcn.Id) {
		return nil, ErrOCIOwnership
	}
	if tags[managedTag] != "" || tags[accessTag] != "" || tags[networkTag] != "" {
		if p.ownChild(compartmentID, vcnID, tags, vcn) != nil {
			return nil, ErrOCIOwnership
		}
	}
	result := make(map[string]string, len(tags)+5)
	for k, v := range tags {
		result[k] = v
	}
	for k, v := range p.networkTags(vcn.FreeformTags[networkTag], kind, childOperationToken(token, kind)) {
		result[k] = v
	}
	return result, nil
}

func (p *OCINetworkProvider) initializeDefaults(ctx context.Context, vcn core.Vcn, source, token string) error {
	if value(vcn.DefaultRouteTableId) == "" || value(vcn.DefaultSecurityListId) == "" || value(vcn.DefaultDhcpOptionsId) == "" {
		return errors.New("OCI VCN is missing default resource IDs")
	}
	route, err := p.network.GetRouteTable(ociRequestContext(ctx), core.GetRouteTableRequest{RtId: vcn.DefaultRouteTableId})
	if err != nil {
		return fmt.Errorf("inspect OCI default route table: %w", err)
	}
	if value(route.Id) != value(vcn.DefaultRouteTableId) {
		return ErrOCIOwnership
	}
	tags, err := p.defaultTags(vcn, route.CompartmentId, route.VcnId, route.FreeformTags, "route-table", token)
	if err != nil {
		return err
	}
	match, err := etag(route.Etag)
	if err != nil {
		return err
	}
	if _, err = p.network.UpdateRouteTable(ociRequestContext(ctx), core.UpdateRouteTableRequest{RtId: route.Id, IfMatch: match, UpdateRouteTableDetails: core.UpdateRouteTableDetails{FreeformTags: tags, RouteRules: []core.RouteRule{}}}); err != nil {
		return fmt.Errorf("initialize OCI default route table: %w", err)
	}
	security, err := p.network.GetSecurityList(ociRequestContext(ctx), core.GetSecurityListRequest{SecurityListId: vcn.DefaultSecurityListId})
	if err != nil {
		return fmt.Errorf("inspect OCI default security list: %w", err)
	}
	if value(security.Id) != value(vcn.DefaultSecurityListId) {
		return ErrOCIOwnership
	}
	tags, err = p.defaultTags(vcn, security.CompartmentId, security.VcnId, security.FreeformTags, "security-list", token)
	if err != nil {
		return err
	}
	match, err = etag(security.Etag)
	if err != nil {
		return err
	}
	if _, err = p.network.UpdateSecurityList(ociRequestContext(ctx), core.UpdateSecurityListRequest{SecurityListId: security.Id, IfMatch: match, UpdateSecurityListDetails: core.UpdateSecurityListDetails{FreeformTags: tags, IngressSecurityRules: sshIngress(source), EgressSecurityRules: []core.EgressSecurityRule{{Protocol: common.String("all"), Destination: common.String("0.0.0.0/0"), DestinationType: core.EgressSecurityRuleDestinationTypeCidrBlock}}}}); err != nil {
		return fmt.Errorf("configure OCI SSH security rules: %w", err)
	}
	dhcp, err := p.network.GetDhcpOptions(ociRequestContext(ctx), core.GetDhcpOptionsRequest{DhcpId: vcn.DefaultDhcpOptionsId})
	if err != nil {
		return fmt.Errorf("inspect OCI default DHCP options: %w", err)
	}
	if value(dhcp.Id) != value(vcn.DefaultDhcpOptionsId) {
		return ErrOCIOwnership
	}
	tags, err = p.defaultTags(vcn, dhcp.CompartmentId, dhcp.VcnId, dhcp.FreeformTags, "dhcp-options", token)
	if err != nil {
		return err
	}
	match, err = etag(dhcp.Etag)
	if err != nil {
		return err
	}
	if _, err = p.network.UpdateDhcpOptions(ociRequestContext(ctx), core.UpdateDhcpOptionsRequest{DhcpId: dhcp.Id, IfMatch: match, UpdateDhcpDetails: core.UpdateDhcpDetails{FreeformTags: tags}}); err != nil {
		return fmt.Errorf("tag OCI default DHCP options: %w", err)
	}
	return nil
}

func (p *OCINetworkProvider) UpdateNetwork(ctx context.Context, id, sshSource string) (domain.Network, error) {
	if err := domain.ValidateSSHSource(sshSource); err != nil {
		return domain.Network{}, err
	}
	vcn, err := p.getOwnedVcn(ctx, id)
	if err != nil {
		return domain.Network{}, err
	}
	if vcn.LifecycleState != core.VcnLifecycleStateAvailable {
		return p.fromVcn(vcn.Vcn), errors.New("OCI network must be available before changing SSH ingress")
	}
	if _, _, err := p.networkChildren(ctx, vcn.Vcn); err != nil {
		return p.fromVcn(vcn.Vcn), err
	}
	security, err := p.network.GetSecurityList(ociRequestContext(ctx), core.GetSecurityListRequest{SecurityListId: vcn.DefaultSecurityListId})
	if err != nil {
		return p.fromVcn(vcn.Vcn), fmt.Errorf("inspect OCI network security list: %w", err)
	}
	if value(security.Id) != value(vcn.DefaultSecurityListId) || p.ownChild(security.CompartmentId, security.VcnId, security.FreeformTags, vcn.Vcn) != nil {
		return p.fromVcn(vcn.Vcn), ErrOCIOwnership
	}
	match, err := etag(security.Etag)
	if err != nil {
		return p.fromVcn(vcn.Vcn), err
	}
	_, err = p.network.UpdateSecurityList(ociRequestContext(ctx), core.UpdateSecurityListRequest{SecurityListId: security.Id, IfMatch: match, UpdateSecurityListDetails: core.UpdateSecurityListDetails{IngressSecurityRules: sshIngress(sshSource)}})
	if err != nil {
		return p.fromVcn(vcn.Vcn), fmt.Errorf("update OCI SSH security rules: %w", err)
	}
	return p.GetNetwork(ctx, id)
}

func (p *OCINetworkProvider) WaitNetwork(ctx context.Context, id, status string) (domain.Network, error) {
	if status != "available" && status != domain.StatusDeleted {
		return domain.Network{}, errors.New("network wait status must be available or deleted")
	}
	ctx, cancel := boundedOCIContext(ctx)
	defer cancel()
	var item domain.Network
	var owned bool
	err := pollOCI(ctx, p.pollInterval, func() (bool, error) {
		resp, err := p.getOwnedVcn(ctx, id)
		if err != nil {
			if owned && status == domain.StatusDeleted && isOCINotFound(err) {
				item.Status = domain.StatusDeleted
				return true, nil
			}
			return false, err
		}
		owned = true
		item = p.fromVcn(resp.Vcn)
		if resp.LifecycleState == core.VcnLifecycleStateTerminated {
			item.Status = domain.StatusDeleted
			if status != domain.StatusDeleted {
				return false, errors.New("OCI VCN terminated while waiting for availability")
			}
		}
		if item.Status == status {
			if status == "available" {
				item, err = p.GetNetwork(ctx, id)
				if err != nil {
					return false, err
				}
				if item.SubnetID == "" || item.InternetGatewayID == "" {
					return false, errors.New("OCI network is missing its managed subnet or gateway")
				}
				subnet, err := p.network.GetSubnet(ociRequestContext(ctx), core.GetSubnetRequest{SubnetId: common.String(item.SubnetID)})
				if err != nil {
					return false, err
				}
				if p.ownChild(subnet.CompartmentId, subnet.VcnId, subnet.FreeformTags, resp.Vcn) != nil {
					return false, ErrOCIOwnership
				}
				gateway, err := p.network.GetInternetGateway(ociRequestContext(ctx), core.GetInternetGatewayRequest{IgId: common.String(item.InternetGatewayID)})
				if err != nil {
					return false, err
				}
				if p.ownChild(gateway.CompartmentId, gateway.VcnId, gateway.FreeformTags, resp.Vcn) != nil {
					return false, ErrOCIOwnership
				}
				return subnet.LifecycleState == core.SubnetLifecycleStateAvailable && gateway.LifecycleState == core.InternetGatewayLifecycleStateAvailable, nil
			}
			return true, nil
		}
		return false, nil
	})
	return item, err
}

func (p *OCINetworkProvider) waitVcn(ctx context.Context, id string, state core.VcnLifecycleStateEnum) error {
	return pollOCI(ctx, p.pollInterval, func() (bool, error) {
		resp, err := p.getOwnedVcn(ctx, id)
		if isOCINotFound(err) && state == core.VcnLifecycleStateTerminated {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if resp.LifecycleState == core.VcnLifecycleStateTerminated && state != core.VcnLifecycleStateTerminated {
			return false, errors.New("OCI VCN terminated before becoming available")
		}
		return resp.LifecycleState == state, nil
	})
}

func (p *OCINetworkProvider) waitSubnet(ctx context.Context, id string, state core.SubnetLifecycleStateEnum) error {
	return pollOCI(ctx, p.pollInterval, func() (bool, error) {
		resp, err := p.network.GetSubnet(ociRequestContext(ctx), core.GetSubnetRequest{SubnetId: common.String(id)})
		if isOCINotFound(err) && state == core.SubnetLifecycleStateTerminated {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if value(resp.Id) != id || ownOCIResource(p.profile, resp.CompartmentId, resp.FreeformTags) != nil {
			return false, ErrOCIOwnership
		}
		return resp.LifecycleState == state, nil
	})
}

func (p *OCINetworkProvider) waitGateway(ctx context.Context, id string, state core.InternetGatewayLifecycleStateEnum) error {
	return pollOCI(ctx, p.pollInterval, func() (bool, error) {
		resp, err := p.network.GetInternetGateway(ociRequestContext(ctx), core.GetInternetGatewayRequest{IgId: common.String(id)})
		if isOCINotFound(err) && state == core.InternetGatewayLifecycleStateTerminated {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if value(resp.Id) != id || ownOCIResource(p.profile, resp.CompartmentId, resp.FreeformTags) != nil {
			return false, ErrOCIOwnership
		}
		return resp.LifecycleState == state, nil
	})
}
