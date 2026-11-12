package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func (p *OCINetworkProvider) DeleteNetwork(ctx context.Context, id string) error {
	ctx, cancel := boundedOCIContext(ctx)
	defer cancel()
	return p.deleteNetwork(ctx, id, false)
}

func (p *OCINetworkProvider) deleteNetwork(ctx context.Context, id string, allowFreshDefaults bool) error {
	vcn, err := p.getOwnedVcn(ctx, id)
	if err != nil {
		return err
	}
	if vcn.LifecycleState == core.VcnLifecycleStateTerminated {
		return nil
	}
	subnets, gateways, err := p.networkChildren(ctx, vcn.Vcn)
	if err != nil {
		return err
	}
	if err := p.checkNetworkDependencies(ctx, vcn.Vcn, allowFreshDefaults); err != nil {
		return err
	}
	for _, subnet := range subnets {
		var page *string
		for {
			ips, err := p.network.ListPrivateIps(ociRequestContext(ctx), core.ListPrivateIpsRequest{SubnetId: subnet.Id, Page: page})
			if err != nil {
				return fmt.Errorf("inspect OCI subnet dependencies: %w", err)
			}
			if len(ips.Items) != 0 {
				return errors.New("OCI subnet contains private IPs or VNICs; delete their instances first")
			}
			page = ips.OpcNextPage
			if value(page) == "" {
				break
			}
		}
	}
	for _, subnet := range subnets {
		current, err := p.network.GetSubnet(ociRequestContext(ctx), core.GetSubnetRequest{SubnetId: subnet.Id})
		if err != nil {
			return fmt.Errorf("inspect OCI subnet before deletion: %w", err)
		}
		if value(current.Id) != value(subnet.Id) || p.ownChild(current.CompartmentId, current.VcnId, current.FreeformTags, vcn.Vcn) != nil {
			return ErrOCIOwnership
		}
		match, err := etag(current.Etag)
		if err != nil {
			return err
		}
		if _, err := p.network.DeleteSubnet(ociRequestContext(ctx), core.DeleteSubnetRequest{SubnetId: current.Id, IfMatch: match}); err != nil {
			return fmt.Errorf("delete OCI subnet: %w", err)
		}
		if err := p.waitSubnet(ctx, value(current.Id), core.SubnetLifecycleStateTerminated); err != nil {
			return fmt.Errorf("wait for OCI subnet deletion: %w", err)
		}
	}
	route, err := p.network.GetRouteTable(ociRequestContext(ctx), core.GetRouteTableRequest{RtId: vcn.DefaultRouteTableId})
	if err != nil {
		return fmt.Errorf("inspect OCI route table before deletion: %w", err)
	}
	if value(route.Id) != value(vcn.DefaultRouteTableId) || p.validateDefaultChild(vcn.Vcn, route.CompartmentId, route.VcnId, route.FreeformTags, allowFreshDefaults) != nil {
		return ErrOCIOwnership
	}
	match, err := etag(route.Etag)
	if err != nil {
		return err
	}
	if _, err := p.network.UpdateRouteTable(ociRequestContext(ctx), core.UpdateRouteTableRequest{RtId: route.Id, IfMatch: match, UpdateRouteTableDetails: core.UpdateRouteTableDetails{RouteRules: []core.RouteRule{}}}); err != nil {
		return fmt.Errorf("clear OCI network routes: %w", err)
	}
	for _, gateway := range gateways {
		current, err := p.network.GetInternetGateway(ociRequestContext(ctx), core.GetInternetGatewayRequest{IgId: gateway.Id})
		if err != nil {
			return fmt.Errorf("inspect OCI gateway before deletion: %w", err)
		}
		if value(current.Id) != value(gateway.Id) || p.ownChild(current.CompartmentId, current.VcnId, current.FreeformTags, vcn.Vcn) != nil {
			return ErrOCIOwnership
		}
		match, err := etag(current.Etag)
		if err != nil {
			return err
		}
		if _, err := p.network.DeleteInternetGateway(ociRequestContext(ctx), core.DeleteInternetGatewayRequest{IgId: current.Id, IfMatch: match}); err != nil {
			return fmt.Errorf("delete OCI internet gateway: %w", err)
		}
		if err := p.waitGateway(ctx, value(current.Id), core.InternetGatewayLifecycleStateTerminated); err != nil {
			return fmt.Errorf("wait for OCI gateway deletion: %w", err)
		}
	}
	current, err := p.getOwnedVcn(ctx, id)
	if err != nil {
		return err
	}
	match, err = etag(current.Etag)
	if err != nil {
		return err
	}
	if _, err := p.network.DeleteVcn(ociRequestContext(ctx), core.DeleteVcnRequest{VcnId: current.Id, IfMatch: match}); err != nil {
		return fmt.Errorf("delete OCI VCN: %w", err)
	}
	if err := p.waitVcn(ctx, id, core.VcnLifecycleStateTerminated); err != nil {
		return fmt.Errorf("wait for OCI VCN deletion: %w", err)
	}
	return nil
}

func (p *OCINetworkProvider) validateDefaultChild(vcn core.Vcn, compartmentID, vcnID *string, tags map[string]string, allowFresh bool) error {
	if value(compartmentID) != p.profile.CompartmentID || value(vcnID) != value(vcn.Id) {
		return ErrOCIOwnership
	}
	if allowFresh && tags[managedTag] == "" && tags[accessTag] == "" && tags[networkTag] == "" {
		return nil
	}
	return p.ownChild(compartmentID, vcnID, tags, vcn)
}

func (p *OCINetworkProvider) checkNetworkDependencies(ctx context.Context, vcn core.Vcn, allowFresh bool) error {
	var page *string
	for {
		resp, err := p.network.ListRouteTables(ociRequestContext(ctx), core.ListRouteTablesRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Page: page})
		if err != nil {
			return fmt.Errorf("inspect OCI route-table dependencies: %w", err)
		}
		for _, route := range resp.Items {
			if value(route.Id) != value(vcn.DefaultRouteTableId) || p.validateDefaultChild(vcn, route.CompartmentId, route.VcnId, route.FreeformTags, allowFresh) != nil {
				return errors.New("OCI network contains a foreign or additional route table")
			}
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	page = nil
	for {
		resp, err := p.network.ListSecurityLists(ociRequestContext(ctx), core.ListSecurityListsRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Page: page})
		if err != nil {
			return fmt.Errorf("inspect OCI security-list dependencies: %w", err)
		}
		for _, security := range resp.Items {
			if value(security.Id) != value(vcn.DefaultSecurityListId) || p.validateDefaultChild(vcn, security.CompartmentId, security.VcnId, security.FreeformTags, allowFresh) != nil {
				return errors.New("OCI network contains a foreign or additional security list")
			}
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	page = nil
	for {
		resp, err := p.network.ListDhcpOptions(ociRequestContext(ctx), core.ListDhcpOptionsRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Page: page})
		if err != nil {
			return fmt.Errorf("inspect OCI DHCP dependencies: %w", err)
		}
		for _, dhcp := range resp.Items {
			if value(dhcp.Id) != value(vcn.DefaultDhcpOptionsId) || p.validateDefaultChild(vcn, dhcp.CompartmentId, dhcp.VcnId, dhcp.FreeformTags, allowFresh) != nil {
				return errors.New("OCI network contains foreign or additional DHCP options")
			}
		}
		page = resp.OpcNextPage
		if value(page) == "" {
			break
		}
	}
	checks := []func() (bool, error){
		func() (bool, error) {
			resp, err := p.network.ListNatGateways(ociRequestContext(ctx), core.ListNatGatewaysRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Limit: common.Int(1)})
			return len(resp.Items) != 0 || value(resp.OpcNextPage) != "", err
		},
		func() (bool, error) {
			resp, err := p.network.ListServiceGateways(ociRequestContext(ctx), core.ListServiceGatewaysRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Limit: common.Int(1)})
			return len(resp.Items) != 0 || value(resp.OpcNextPage) != "", err
		},
		func() (bool, error) {
			resp, err := p.network.ListLocalPeeringGateways(ociRequestContext(ctx), core.ListLocalPeeringGatewaysRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Limit: common.Int(1)})
			return len(resp.Items) != 0 || value(resp.OpcNextPage) != "", err
		},
		func() (bool, error) {
			resp, err := p.network.ListNetworkSecurityGroups(ociRequestContext(ctx), core.ListNetworkSecurityGroupsRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Limit: common.Int(1)})
			return len(resp.Items) != 0 || value(resp.OpcNextPage) != "", err
		},
		func() (bool, error) {
			resp, err := p.network.ListDrgAttachments(ociRequestContext(ctx), core.ListDrgAttachmentsRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Limit: common.Int(1)})
			return len(resp.Items) != 0 || value(resp.OpcNextPage) != "", err
		},
		func() (bool, error) {
			resp, err := p.network.ListVlans(ociRequestContext(ctx), core.ListVlansRequest{CompartmentId: common.String(p.profile.CompartmentID), VcnId: vcn.Id, Limit: common.Int(1)})
			return len(resp.Items) != 0 || value(resp.OpcNextPage) != "", err
		},
	}
	for _, check := range checks {
		present, err := check()
		if err != nil {
			return fmt.Errorf("inspect OCI network dependencies: %w", err)
		}
		if present {
			return errors.New("OCI network contains unsupported gateway, peering, security-group, or VLAN dependencies")
		}
	}
	return nil
}
