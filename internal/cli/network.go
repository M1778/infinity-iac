package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/M1778/infinity-iac/internal/provider"
)

func (a *App) networkProvider(access string) (provider.NetworkProvider, error) {
	profile, ok := a.cfg.DefaultProfile()
	if access != "" {
		profile, ok = a.cfg.Profile(access)
	}
	if !ok {
		return nil, errors.New("no access profile configured; run setup oracle --network-only")
	}
	if a.n != nil {
		return a.n, nil
	}
	return provider.NewOCINetworkProvider(profile)
}

func (a *App) network(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: network create|list|get|update|delete [ID] [--access NAME] [--name NAME] [--cidr CIDR] [--subnet-cidr CIDR] [--ssh-source CIDR] [--yes] [--timeout 10m] [--output json]")
		return 0
	}
	action := args[0]
	if action != "create" && action != "list" && action != "get" && action != "update" && action != "delete" {
		return a.fail("unknown network action " + action)
	}
	fs := flag.NewFlagSet("network "+action, flag.ContinueOnError)
	fs.SetOutput(a.err)
	access := fs.String("access", "", "access profile")
	name := fs.String("name", "", "network name")
	cidr := fs.String("cidr", "10.88.0.0/16", "VCN IPv4 CIDR")
	subnetCIDR := fs.String("subnet-cidr", "10.88.1.0/24", "subnet IPv4 CIDR")
	sshSource := fs.String("ssh-source", "", "narrow IPv4 source CIDR permitted to connect to SSH")
	yes := fs.Bool("yes", false, "confirm cloud mutation")
	wait := fs.Bool("wait", false, "confirm network completion")
	timeout := fs.Duration("timeout", 10*time.Minute, "operation deadline")
	mode := fs.String("output", "table", "table or json")
	parseArgs, err := flagsBeforePositionals(fs, args[1:])
	if err != nil {
		return a.fail(err.Error())
	}
	if err := fs.Parse(parseArgs); err != nil {
		return 2
	}
	if *mode != "table" && *mode != "json" {
		return a.fail("output must be table or json")
	}
	if *timeout <= 0 || *timeout > time.Hour {
		return a.fail("timeout must be greater than zero and at most one hour")
	}
	if action == "create" || action == "list" {
		if fs.NArg() != 0 {
			return a.fail("network " + action + " accepts flags only")
		}
	} else if fs.NArg() != 1 {
		return a.fail("network ID or name is required")
	}
	profile, ok := a.cfg.DefaultProfile()
	if *access != "" {
		profile, ok = a.cfg.Profile(*access)
	}
	if !ok {
		return a.fail("no access profile configured; run setup oracle --network-only")
	}
	spec := domain.NetworkSpec{Name: *name, Provider: profile.Provider, Access: profile.Name, Region: profile.Region, CIDR: *cidr, SubnetCIDR: *subnetCIDR, SSHSource: *sshSource}
	if action == "create" {
		if err := spec.Validate(); err != nil {
			return a.fail(err.Error())
		}
	}
	if action == "update" {
		if err := domain.ValidateSSHSource(*sshSource); err != nil {
			return a.fail(err.Error())
		}
	}
	if action == "create" || action == "update" || action == "delete" {
		if !*yes && !isTerminal(a.in) {
			return a.fail("network mutation requires --yes in noninteractive mode")
		}
		if !*yes && !a.confirm(action+" managed network") {
			return 1
		}
	}
	backend, err := a.networkProvider(profile.Name)
	if err != nil {
		return a.fail(err.Error())
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if action == "list" {
		items, err := backend.ListNetworks(ctx)
		if err != nil {
			return a.fail(err.Error())
		}
		if *mode == "json" {
			return a.emit(items, nil)
		}
		for _, item := range items {
			fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Status, item.CIDR, item.SubnetID)
		}
		return 0
	}
	id := ""
	if action != "create" {
		items, err := backend.ListNetworks(ctx)
		if err != nil {
			return a.fail(err.Error())
		}
		for _, item := range items {
			if item.ID == fs.Arg(0) || item.Name == fs.Arg(0) {
				if id != "" && id != item.ID {
					return a.fail("network name is ambiguous; use an exact ID")
				}
				id = item.ID
			}
		}
		if id == "" {
			return a.fail(fmt.Sprintf("managed network %q not found", fs.Arg(0)))
		}
	}
	var item domain.Network
	switch action {
	case "create":
		token := make([]byte, 16)
		if _, err := rand.Read(token); err != nil {
			return a.fail(err.Error())
		}
		item, err = backend.CreateNetwork(ctx, provider.NetworkCreateRequest{Spec: spec, IdempotencyKey: "network-" + hex.EncodeToString(token)})
	case "get":
		item, err = backend.GetNetwork(ctx, id)
	case "update":
		item, err = backend.UpdateNetwork(ctx, id, *sshSource)
	case "delete":
		err = backend.DeleteNetwork(ctx, id)
		if err == nil {
			if *mode == "json" {
				return a.emit(map[string]any{"id": id, "status": "deleted", "confirmed": true}, nil)
			}
			fmt.Fprintln(a.out, "deleted", id)
			return 0
		}
	}
	if err != nil {
		if item.ID != "" {
			fmt.Fprintf(a.err, "partial managed network %s; inspect and delete this exact ID after resolving the failure\n", item.ID)
		}
		return a.fail(err.Error())
	}
	if *wait && action != "get" {
		if waiter, ok := backend.(interface {
			WaitNetwork(context.Context, string, string) (domain.Network, error)
		}); ok {
			item, err = waiter.WaitNetwork(ctx, item.ID, "available")
			if err != nil {
				return a.fail(err.Error())
			}
		}
	}
	if *mode == "json" {
		return a.emit(item, nil)
	}
	fmt.Fprintf(a.out, "%s %s (%s), subnet %s\n", action, item.Name, item.ID, item.SubnetID)
	return 0
}
