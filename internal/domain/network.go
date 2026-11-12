package domain

import (
	"errors"
	"net/netip"
	"regexp"
	"strings"
)

var networkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

type NetworkSpec struct {
	Name       string `json:"name" yaml:"name"`
	Provider   string `json:"provider" yaml:"provider"`
	Access     string `json:"access,omitempty" yaml:"access,omitempty"`
	Region     string `json:"region,omitempty" yaml:"region,omitempty"`
	CIDR       string `json:"cidr" yaml:"cidr"`
	SubnetCIDR string `json:"subnet_cidr" yaml:"subnet_cidr"`
	SSHSource  string `json:"ssh_source" yaml:"ssh_source"`
}

func (s NetworkSpec) Validate() error {
	if !networkNamePattern.MatchString(s.Name) {
		return errors.New("network name must be 1-100 letters, digits, dots, underscores, or hyphens, starting with a letter or digit")
	}
	if s.Provider != "oracle" && s.Provider != "oci" {
		return errors.New("network provider must be oracle or oci")
	}
	vcn, err := netip.ParsePrefix(s.CIDR)
	if err != nil || !vcn.Addr().Is4() || vcn != vcn.Masked() || vcn.Bits() < 16 || vcn.Bits() > 30 || !vcn.Addr().IsPrivate() {
		return errors.New("network CIDR must be a canonical private IPv4 prefix between /16 and /30")
	}
	subnet, err := netip.ParsePrefix(s.SubnetCIDR)
	if err != nil || !subnet.Addr().Is4() || subnet != subnet.Masked() || subnet.Bits() < vcn.Bits() || subnet.Bits() > 30 || !vcn.Contains(subnet.Addr()) {
		return errors.New("subnet CIDR must be a canonical IPv4 prefix contained in the network, no smaller than /30")
	}
	return ValidateSSHSource(s.SSHSource)
}

func ValidateSSHSource(source string) error {
	prefix, err := netip.ParsePrefix(source)
	if err != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() || prefix.Bits() < 24 || !prefix.Addr().IsGlobalUnicast() {
		return errors.New("SSH source must be a canonical IPv4 /24-/32 prefix; use your own address with /32")
	}
	return nil
}

type Network struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Provider          string `json:"provider"`
	Access            string `json:"access"`
	Region            string `json:"region"`
	CIDR              string `json:"cidr"`
	SubnetCIDR        string `json:"subnet_cidr,omitempty"`
	SubnetID          string `json:"subnet_id,omitempty"`
	InternetGatewayID string `json:"internet_gateway_id,omitempty"`
	RouteTableID      string `json:"route_table_id,omitempty"`
	SecurityListID    string `json:"security_list_id,omitempty"`
	SSHSource         string `json:"ssh_source,omitempty"`
	Status            string `json:"status"`
}

func (s NetworkSpec) ResolveDefaults(access, region string) NetworkSpec {
	if s.Provider == "" {
		s.Provider = "oracle"
	}
	if s.Access == "" {
		s.Access = access
	}
	if s.Region == "" {
		s.Region = region
	}
	s.SSHSource = strings.TrimSpace(s.SSHSource)
	return s
}
