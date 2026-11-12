package domain

import "testing"

func TestNetworkSpecValidation(t *testing.T) {
	valid := NetworkSpec{Name: "test-network", Provider: "oracle", CIDR: "10.88.0.0/16", SubnetCIDR: "10.88.42.0/24", SSHSource: "192.0.2.10/32"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*NetworkSpec){
		"missing-name":     func(s *NetworkSpec) { s.Name = "" },
		"invalid-name":     func(s *NetworkSpec) { s.Name = "../personal" },
		"provider":         func(s *NetworkSpec) { s.Provider = "local" },
		"public-vcn":       func(s *NetworkSpec) { s.CIDR = "8.8.0.0/16" },
		"unaligned-vcn":    func(s *NetworkSpec) { s.CIDR = "10.88.1.0/16" },
		"overbroad-vcn":    func(s *NetworkSpec) { s.CIDR = "10.0.0.0/8" },
		"foreign-subnet":   func(s *NetworkSpec) { s.SubnetCIDR = "10.89.0.0/24" },
		"overbroad-subnet": func(s *NetworkSpec) { s.SubnetCIDR = "10.0.0.0/8" },
		"unaligned-subnet": func(s *NetworkSpec) { s.SubnetCIDR = "10.88.42.1/24" },
		"ipv6-subnet":      func(s *NetworkSpec) { s.SubnetCIDR = "fd00::/64" },
		"open-ssh":         func(s *NetworkSpec) { s.SSHSource = "0.0.0.0/0" },
		"overbroad-ssh":    func(s *NetworkSpec) { s.SSHSource = "192.0.0.0/16" },
		"unaligned-ssh":    func(s *NetworkSpec) { s.SSHSource = "192.0.2.10/24" },
		"multicast-ssh":    func(s *NetworkSpec) { s.SSHSource = "224.0.0.1/32" },
		"no-cidr-ssh":      func(s *NetworkSpec) { s.SSHSource = "192.0.2.10" },
		"too-small-subnet": func(s *NetworkSpec) { s.SubnetCIDR = "10.88.42.0/31" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			spec := valid
			change(&spec)
			if err := spec.Validate(); err == nil {
				t.Fatal("invalid network specification accepted")
			}
		})
	}
}

func TestNetworkSpecDefaults(t *testing.T) {
	spec := (NetworkSpec{Name: "network", SSHSource: " 192.0.2.10/32 "}).ResolveDefaults("access", "region")
	if spec.Provider != "oracle" || spec.Access != "access" || spec.Region != "region" || spec.SSHSource != "192.0.2.10/32" {
		t.Fatalf("unexpected defaults: %+v", spec)
	}
}
