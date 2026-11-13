package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/config"
	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/M1778/infinity-iac/internal/provider"
	"github.com/M1778/infinity-iac/internal/recipe"
	builtinrecipes "github.com/M1778/infinity-iac/recipes"
)

type App struct {
	in         io.Reader
	out, err   io.Writer
	cfg        config.File
	configPath string
	p          provider.Provider
	n          provider.NetworkProvider
}

var Version = "1.1.1"

func Run(args []string) int { return RunWithIO(args, os.Stdin, os.Stdout, os.Stderr) }
func RunWithIO(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) > 0 && args[0] == "__complete" {
		return complete(args[1:], out)
	}
	cfg, path, err := config.Load()
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	a := &App{in: in, out: out, err: errOut, cfg: cfg, configPath: path}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		fmt.Fprintln(out, Version)
		return 0
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		a.help()
		return 0
	}
	switch args[0] {
	case "setup":
		return a.setup(args[1:])
	case "unlink":
		return a.unlink(args[1:])
	case "list":
		return a.list(args[1:])
	case "recipe":
		return a.recipe(args[1:])
	case "validate":
		return a.validate(args[1:])
	case "create":
		return a.create(args[1:])
	case "run":
		return a.run(args[1:])
	case "network":
		return a.network(args[1:])
	case "oc-hunter":
		return a.ocHunter(args[1:])
	case "instance":
		return a.instance(args[1:])
	case "doctor":
		return a.doctor(args[1:])
	case "plan":
		return a.plan(args[1:], false)
	case "apply":
		return a.plan(args[1:], true)
	case "install":
		return a.install(args[1:])
	case "app":
		return a.app(args[1:])
	case "update":
		return a.update(args[1:])
	case "completion":
		return a.completion(args[1:])
	default:
		fmt.Fprintf(a.err, "error: unknown command %q\n", args[0])
		return 2
	}
}
func (a *App) help() {
	fmt.Fprintln(a.out, "Infinity-IaC: infrastructure lifecycle CLI")
	fmt.Fprintln(a.out, "commands: setup, unlink, list, create, run, network, instance, oc-hunter, plan, apply, install, app, update, recipe, validate, completion, doctor")
	fmt.Fprintln(a.out, "examples:")
	fmt.Fprintln(a.out, "  infinity-iac setup oracle --name default --profile DEFAULT --region us-phoenix-1 --compartment OCID --subnet OCID --availability-domain AD --image OCID --shape SHAPE")
	fmt.Fprintln(a.out, "  infinity-iac create server --name demo --provider oracle --yes")
	fmt.Fprintln(a.out, "  infinity-iac list instances --output json")
}

func (a *App) completion(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(a.out, "usage: completion bash|zsh|fish|powershell")
		return 0
	}
	script, ok := completionScript(args[0])
	if !ok {
		return a.fail("supported shells: bash, zsh, fish, powershell")
	}
	fmt.Fprint(a.out, script)
	return 0
}

func (a *App) setup(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: setup oracle --compartment OCID --subnet OCID --availability-domain AD --image OCID --shape SHAPE [--name NAME] [--profile PROFILE] [--region REGION] [--config PATH] [--autodetect] [--ssh-public-key KEY]")
		return 0
	}
	if args[0] != "oracle" {
		return a.fail("only the oracle access guide is available in this release")
	}
	fs := flag.NewFlagSet("setup oracle", flag.ContinueOnError)
	fs.SetOutput(a.err)
	name := fs.String("name", "", "access profile name")
	profile := fs.String("profile", "DEFAULT", "provider credential profile")
	account := fs.String("account", "", "account or tenancy identifier")
	region := fs.String("region", "", "default region")
	configFile := fs.String("config", "", "OCI config file path")
	compartment := fs.String("compartment", "", "OCI compartment OCID")
	subnet := fs.String("subnet", "", "OCI subnet OCID")
	availabilityDomain := fs.String("availability-domain", "", "OCI availability domain")
	image := fs.String("image", "", "OCI image OCID")
	shape := fs.String("shape", "", "OCI compute shape")
	sshKey := fs.String("ssh-public-key", "", "SSH public key")
	networkOnly := fs.Bool("network-only", false, "validate credentials and compartment for network management without a compute profile")
	autodetect := fs.Bool("autodetect", false, "automatically detect OCI configuration")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return a.fail("setup oracle accepts flags only")
	}
	if *autodetect {
		metadata, err := provider.DetectOCIConfiguration(*configFile, *profile)
		if err != nil {
			return a.fail("autodetect OCI config: " + err.Error())
		}
		*configFile = metadata.Path
		if *region == "" && os.Getenv("OCI_REGION") == "" {
			*region = metadata.Region
		}
		if *account == "" {
			*account = metadata.TenancyID
		}
	}
	if *name == "" {
		*name = "default"
	}
	if *region == "" {
		*region = os.Getenv("OCI_REGION")
	}
	if *region == "" {
		return a.fail("--region is required")
	}
	if *profile == "" {
		*profile = "DEFAULT"
	}
	p := config.NewProfile(*name, "oracle", "oci-profile:"+*profile, *account, *region)
	p.ConfigPath = *configFile
	p.CompartmentID = *compartment
	p.SubnetID = *subnet
	p.AvailabilityDomain = *availabilityDomain
	p.ImageID = *image
	p.Shape = *shape
	p.SSHPublicKey = *sshKey
	if !*networkOnly {
		if err := validateAccessProfile(p); err != nil {
			return a.fail(err.Error())
		}
	}
	checkCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if *networkOnly {
		backend, err := provider.NewOCINetworkProvider(p)
		if err != nil {
			return a.fail("validate OCI credentials: " + err.Error())
		}
		if err := backend.ValidateAccess(checkCtx); err != nil {
			return a.fail("validate OCI network access: " + err.Error())
		}
	} else {
		backend, err := provider.NewOCIProvider(p)
		if err != nil {
			return a.fail("validate OCI credentials: " + err.Error())
		}
		if err := backend.ValidateAccess(checkCtx); err != nil {
			return a.fail("validate OCI access: " + err.Error())
		}
	}
	p.ValidatedAt = time.Now().UTC()
	a.cfg.UpsertProfile(p)
	if err := config.Save(a.configPath, a.cfg); err != nil {
		return a.fail("save config: " + err.Error())
	}
	return a.emit(map[string]any{"profile": p, "config": a.configPath, "validation": "OCI credentials and required resources validated with read-only API calls"}, func() {
		fmt.Fprintf(a.out, "configured OCI access %q (%s); credentials and required resources validated\n", p.Name, p.CredentialSource)
	})
}

func validateAccessProfile(p domain.AccessProfile) error {
	if p.CompartmentID == "" || p.SubnetID == "" || p.AvailabilityDomain == "" || p.ImageID == "" || p.Shape == "" {
		return errors.New("--compartment, --subnet, --availability-domain, --image, and --shape are required")
	}
	return nil
}

func (a *App) unlink(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: unlink acc NAME [--yes] [--output table|json]")
		return 0
	}
	if args[0] != "acc" && args[0] != "access" {
		return a.fail("unknown unlink target " + args[0])
	}
	var name string
	parseArgs := make([]string, 0, len(args[1:]))
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			parseArgs = append(parseArgs, arg)
		} else if name == "" {
			name = arg
		} else {
			parseArgs = append(parseArgs, arg)
		}
	}
	fs := flag.NewFlagSet("unlink acc", flag.ContinueOnError)
	fs.SetOutput(a.err)
	yes := fs.Bool("yes", false, "confirm local disconnect")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(parseArgs); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return a.fail("unlink accepts one access profile name")
	}
	if name == "" {
		return a.fail("access profile name is required")
	}
	if *mode != "table" && *mode != "json" {
		return a.fail("output must be table or json")
	}
	if _, ok := a.cfg.Profile(name); !ok {
		return a.fail(fmt.Sprintf("access profile %q is not configured", name))
	}
	if !*yes && !isTerminal(a.in) {
		return a.fail("unlink requires --yes in noninteractive mode")
	}
	if !*yes && !a.confirm("unlink local access profile "+name) {
		return 1
	}
	a.cfg.RemoveProfile(name)
	if err := config.Save(a.configPath, a.cfg); err != nil {
		return a.fail("save config: " + err.Error())
	}
	result := map[string]any{"name": name, "status": "unlinked", "config": a.configPath, "remote": "unchanged"}
	if strings.EqualFold(*mode, "json") {
		return a.emit(result, nil)
	}
	fmt.Fprintf(a.out, "unlinked local access profile %q; OCI credentials and resources unchanged\n", name)
	return 0
}

func (a *App) provider() (provider.Provider, error) {
	if a.p != nil {
		return a.p, nil
	}
	profile, ok := a.cfg.DefaultProfile()
	if !ok {
		return nil, errors.New("no access profile configured; run setup oracle")
	}
	p, err := a.providerForProfile(profile)
	if err != nil {
		return nil, err
	}
	a.p = p
	return p, nil
}

func (a *App) providerForProfile(profile domain.AccessProfile) (provider.Provider, error) {
	if a.p != nil {
		if configured, ok := a.p.(interface{ Profile() domain.AccessProfile }); !ok || configured.Profile() == profile {
			return a.p, nil
		}
	}
	switch profile.Provider {
	case "oracle", "oci":
		p, err := provider.NewOCIProvider(profile)
		if err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", profile.Provider)
	}
}

func (a *App) list(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: list acc|instances [--output table|json]")
		return 0
	}
	switch args[0] {
	case "acc", "access":
		return a.listAccess(args[1:])
	case "instances":
		return a.listInstances(args[1:])
	case "networks":
		return a.network(append([]string{"list"}, args[1:]...))
	case "recipes":
		return a.listRecipes(args[1:])
	case "capabilities":
		return a.listCapabilities(args[1:])
	default:
		return a.fail("unknown list target " + args[0])
	}
}

type recipeSummary struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Kind         string   `json:"kind"`
	Execution    string   `json:"execution"`
	Description  string   `json:"description,omitempty"`
	Status       string   `json:"status,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

func (a *App) listRecipes(args []string) int {
	mode, err := outputMode(args)
	if err != nil {
		return a.fail(err.Error())
	}
	root := recipeRoot(args)
	items, source, err := loadRecipes(root)
	if err != nil {
		return a.fail(err.Error())
	}
	summaries := make([]recipeSummary, 0, len(items))
	for _, item := range items {
		execution := item.Execution
		if execution == "" {
			execution = "instance"
		}
		summaries = append(summaries, recipeSummary{Name: item.Name, Version: item.Version, Kind: item.Kind, Execution: execution, Description: item.Description, Status: item.Status, Requirements: item.Requirements})
	}
	if mode == "json" {
		return a.emit(map[string]any{"source": source, "recipes": summaries}, nil)
	}
	if len(summaries) == 0 {
		fmt.Fprintln(a.out, "no recipes")
		return 0
	}
	fmt.Fprintln(a.out, "NAME\tVERSION\tKIND\tSTATUS\tDESCRIPTION")
	for _, item := range summaries {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%s\n", item.Name, item.Version, item.Kind, item.Status, item.Description)
	}
	return 0
}

func (a *App) listCapabilities(args []string) int {
	root := recipeRoot(args)
	items, source, err := loadRecipes(root)
	if err != nil {
		return a.fail(err.Error())
	}
	capabilities := make([]map[string]any, 0, len(items))
	for _, item := range items {
		capabilities = append(capabilities, map[string]any{"name": item.Name, "kind": item.Kind, "execution": item.Execution, "providers": item.Supports.Providers, "os": item.Supports.OS, "architectures": item.Supports.Architectures, "status": item.Status, "requirements": item.Requirements})
	}
	mode, err := outputMode(args)
	if err != nil {
		return a.fail(err.Error())
	}
	if mode == "json" {
		return a.emit(map[string]any{"source": source, "capabilities": capabilities}, nil)
	}
	for _, item := range capabilities {
		fmt.Fprintf(a.out, "%s\t%s\n", item["name"], item["kind"])
	}
	return 0
}

func (a *App) recipe(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: recipe list|validate|render ...")
		return 0
	}
	switch args[0] {
	case "list":
		return a.listRecipes(args[1:])
	case "validate":
		return a.validateRecipe(args[1:])
	case "render":
		return a.renderRecipe(args[1:])
	default:
		return a.fail("unknown recipe action " + args[0])
	}
}

func (a *App) validate(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: validate recipe --file PATH")
		return 0
	}
	if args[0] != "recipe" {
		return a.fail("unknown validation target " + args[0])
	}
	return a.validateRecipe(args[1:])
}

func (a *App) validateRecipe(args []string) int {
	fs := flag.NewFlagSet("validate recipe", flag.ContinueOnError)
	fs.SetOutput(a.err)
	file := fs.String("file", "", "recipe YAML or JSON file")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *file == "" {
		return a.fail("--file is required")
	}
	item, err := recipe.Load(*file)
	if err != nil {
		return a.fail(err.Error())
	}
	if *mode == "json" {
		return a.emit(map[string]any{"valid": true, "name": item.Name, "version": item.Version, "kind": item.Kind, "file": *file}, nil)
	}
	fmt.Fprintf(a.out, "valid recipe %s %s (%s)\n", item.Name, item.Version, item.Kind)
	return 0
}

func loadRecipes(root string) ([]recipe.Recipe, string, error) {
	var (
		items  []recipe.Recipe
		source string
		err    error
	)
	switch {
	case root != "":
		source = root
		items, err = recipe.Discover(source)
	case os.Getenv("INFINITY_IAC_RECIPES") != "":
		source = os.Getenv("INFINITY_IAC_RECIPES")
		items, err = recipe.Discover(source)
	default:
		source = "builtin"
		items, err = recipe.DiscoverFS(builtinrecipes.FS)
	}
	if err != nil {
		return nil, source, err
	}
	// Installed recipes are part of the normal user catalog. Explicit roots and
	// INFINITY_IAC_RECIPES remain isolated development catalogs.
	if root == "" && os.Getenv("INFINITY_IAC_RECIPES") == "" {
		configPath, pathErr := config.Path()
		if pathErr != nil {
			return nil, source, pathErr
		}
		installed, appErr := recipe.Discover(config.AppsDir(configPath))
		if appErr != nil && !errors.Is(appErr, os.ErrNotExist) {
			return nil, source, fmt.Errorf("discover installed recipes: %w", appErr)
		}
		items = append(items, installed...)
		if len(installed) > 0 {
			source += ", installed"
		}
	}
	catalog, err := recipe.NewCatalog(items)
	if err != nil {
		return nil, source, err
	}
	return catalog.List(), source, nil
}

func recipeRoot(args []string) string {
	for i, arg := range args {
		if arg == "--root" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--root=") {
			return strings.TrimPrefix(arg, "--root=")
		}
	}
	return ""
}
func (a *App) listAccess(args []string) int {
	mode, err := outputMode(args)
	if err != nil {
		return a.fail(err.Error())
	}
	if mode == "json" {
		return a.emit(a.cfg.Profiles, nil)
	}
	if len(a.cfg.Profiles) == 0 {
		fmt.Fprintln(a.out, "no access profiles configured; run setup oracle")
		return 0
	}
	fmt.Fprintln(a.out, "NAME\tPROVIDER\tACCOUNT\tREGION\tDEFAULT")
	for _, p := range a.cfg.Profiles {
		mark := ""
		if p.Name == a.cfg.DefaultAccess {
			mark = "*"
		}
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Provider, p.Account, p.Region, mark)
	}
	return 0
}
func (a *App) listInstances(args []string) int {
	fs := flag.NewFlagSet("list instances", flag.ContinueOnError)
	fs.SetOutput(a.err)
	access := fs.String("access", "", "access profile")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *mode != "table" && *mode != "json" {
		return a.fail("list instances accepts --access and --output table|json")
	}
	profile, ok := a.cfg.DefaultProfile()
	if *access != "" {
		profile, ok = a.cfg.Profile(*access)
	}
	if !ok {
		return a.fail("no access profile configured; run setup oracle")
	}
	p, err := a.providerForProfile(profile)
	if err != nil {
		return a.fail(err.Error())
	}
	a.p = p
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	items, err := p.ListInstances(ctx, provider.Filter{Access: profile.Name})
	if err != nil {
		return a.fail(err.Error())
	}
	if *mode == "json" {
		return a.emit(items, nil)
	}
	if len(items) == 0 {
		fmt.Fprintln(a.out, "no instances")
		return 0
	}
	fmt.Fprintln(a.out, "ID\tNAME\tSTATUS\tPROVIDER\tIP\tSPECS")
	for _, i := range items {
		fmt.Fprintf(a.out, "%s\t%s\t%s\t%s\t%s\t%dvCPU/%dGB/%dGB\n", i.ID, i.Name, i.Status, i.Provider, i.PublicIP, i.CPU, i.RAMGB, i.StorageGB)
	}
	return 0
}

func (a *App) create(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: create server [--name NAME] [--provider PROVIDER] [--cpu N] [--ram NGB] [--storage-size NGB] [--amount N] [--yes]")
		return 0
	}
	if args[0] == "network" {
		return a.network(append([]string{"create"}, args[1:]...))
	}
	if args[0] != "server" {
		return a.fail("unknown create target " + args[0])
	}
	return a.createServer(args[1:], "server")
}
func (a *App) createServer(args []string, kind string, userData ...string) int {
	if len(args) > 0 && args[0] == "help" {
		fmt.Fprintln(a.out, "usage: create server [--name NAME] [--provider PROVIDER] [--cpu N] [--ram NGB] [--storage-size NGB] [--amount N] [--yes]")
		return 0
	}
	fs := flag.NewFlagSet("create server", flag.ContinueOnError)
	fs.SetOutput(a.err)
	name := fs.String("name", "", "server name")
	osName := fs.String("os", "", "operating system image")
	cpu := fs.Int("cpu", 0, "vCPU count")
	ram := fs.String("ram", "", "RAM, for example 2GB")
	storage := fs.String("storage-size", "", "storage, for example 20GB")
	providerName := fs.String("provider", "", "provider name")
	access := fs.String("access", "", "access profile")
	region := fs.String("region", "", "provider region")
	networking := fs.String("networking-script", "", "networking recipe")
	startup := fs.String("startup-script", "", "startup script path")
	port := fs.Int("port", 0, "application port")
	amount := fs.Int("amount", 1, "number of servers")
	yes := fs.Bool("yes", false, "confirm billable operation")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *amount < 1 || *amount > 100 {
		return a.fail("amount must be between 1 and 100")
	}
	if fs.NArg() != 0 {
		return a.fail("create server accepts flags only")
	}
	if *mode != "table" && *mode != "json" {
		return a.fail("output must be table or json")
	}
	if *port < 0 || *port > 65535 {
		return a.fail("port must be between 0 and 65535")
	}
	if strings.TrimSpace(*networking) != "" {
		return a.fail("--networking-script is not supported by the OCI provider in this release")
	}
	profile, ok := a.cfg.DefaultProfile()
	if *access != "" {
		profile, ok = a.cfg.Profile(*access)
	}
	if !ok {
		return a.fail("no access profile configured; run setup oracle")
	}
	if *providerName != "" && *providerName != profile.Provider {
		return a.fail(fmt.Sprintf("provider %q does not match access profile %q", *providerName, profile.Name))
	}
	p, err := a.providerForProfile(profile)
	if err != nil {
		return a.fail(err.Error())
	}
	if *providerName == "" {
		*providerName = profile.Provider
	}
	if *access == "" {
		*access = profile.Name
	}
	if *region == "" {
		*region = profile.Region
	}
	if !*yes && !isTerminal(a.in) {
		return a.fail("creation requires --yes in noninteractive mode")
	}
	if !*yes && !a.confirm("create instance(s)") {
		return 1
	}
	rams, err := sizeGB(*ram, 1)
	if err != nil {
		return a.fail(err.Error())
	}
	storages, err := sizeGB(*storage, 20)
	if err != nil {
		return a.fail(err.Error())
	}
	base := *name
	if base == "" {
		base = kind
	}
	created := make([]domain.Instance, 0, *amount)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	operation := make([]byte, 16)
	if _, err := rand.Read(operation); err != nil {
		return a.fail("create operation identity: " + err.Error())
	}
	for n := 0; n < *amount; n++ {
		nme := base
		if *amount > 1 {
			nme = fmt.Sprintf("%s-%d", base, n+1)
		}
		spec := domain.ServerSpec{Name: nme, OS: *osName, CPU: *cpu, RAMGB: rams, StorageGB: storages, Provider: *providerName, Access: *access, Region: *region, NetworkingScript: *networking, StartupScript: *startup, Port: *port}.ResolveDefaults(*providerName, *access, *region)
		if err := spec.Validate(); err != nil {
			return a.fail(err.Error())
		}
		bootstrap := ""
		if len(userData) > 0 {
			bootstrap = userData[0]
		}
		if *startup != "" {
			script, readErr := os.ReadFile(*startup)
			if readErr != nil {
				return a.fail("read startup script: " + readErr.Error())
			}
			if bootstrap != "" {
				bootstrap += "\n"
			}
			bootstrap += string(script)
		}
		i, err := p.CreateInstance(ctx, provider.CreateRequest{Spec: spec, IdempotencyKey: "create-" + hex.EncodeToString(operation) + "-" + strconv.Itoa(n), UserData: bootstrap})
		if err != nil {
			return a.fail(err.Error())
		}
		created = append(created, i)
	}
	if strings.EqualFold(*mode, "json") {
		return a.emit(created, nil)
	}
	for _, i := range created {
		fmt.Fprintf(a.out, "created %s (%s) at %s\n", i.Name, i.ID, i.PublicIP)
	}
	return 0
}

func (a *App) run(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: run --app NAME|--script NAME|--recipe NAME [--access NAME] [--inputs FILE] [--port N] [--local] [--dry-run] [--yes]")
		return 0
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(a.err)
	app := fs.String("app", "", "application name")
	scriptName := fs.String("script", "", "script name")
	recipeName := fs.String("recipe", "", "recipe name")
	specs := fs.String("specs", "auto", "specification preset")
	port := fs.Int("port", 0, "application port")
	name := fs.String("name", "", "instance name")
	access := fs.String("access", "", "access profile")
	inputsFile := fs.String("inputs", "", "JSON or YAML file containing typed input values")
	startup := fs.String("startup-script", "", "additional startup script path")
	local := fs.Bool("local", false, "confirm execution on this machine for a local script")
	dryRun := fs.Bool("dry-run", false, "validate and describe execution without launching or running commands")
	yes := fs.Bool("yes", false, "confirm execution")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	selectors := 0
	for _, value := range []string{*app, *scriptName, *recipeName} {
		if value != "" {
			selectors++
		}
	}
	if selectors != 1 || fs.NArg() != 0 {
		return a.fail("run requires exactly one of --app, --script, or --recipe")
	}
	if *mode != "table" && *mode != "json" {
		return a.fail("output must be table or json")
	}
	target := *app
	if target == "" {
		target = *scriptName
	}
	if target == "" {
		target = *recipeName
	}
	if *specs != "" && *specs != "auto" {
		return a.fail("only specs=auto is supported in this release")
	}
	item, ok := findRecipe(target)
	if !ok {
		return a.fail(fmt.Sprintf("recipe %q was not found; use list recipes", target))
	}
	if *app != "" && item.Kind != "application" || *scriptName != "" && item.Kind != "script" {
		return a.fail("recipe kind does not match the selected --app or --script selector")
	}
	values, err := loadRecipeInputs(*inputsFile)
	if err != nil {
		return a.fail(err.Error())
	}
	if values == nil {
		values = make(map[string]any)
	}
	if item.Execution == "local" && *access != "" {
		if _, exists := item.Inputs["access"]; exists {
			values["access"] = *access
		}
	}
	if item.Execution == "local" && *name != "" {
		if _, exists := item.Inputs["name"]; exists {
			values["name"] = *name
		}
	}
	if *port == 0 && values["port"] == nil {
		*port = recipePort(item)
	}
	options := recipe.RenderOptions{Port: *port, Inputs: values}
	if item.Execution == "local" {
		if *startup != "" {
			return a.fail("local scripts do not accept a guest startup script")
		}
		if !*local && !*dryRun {
			return a.fail("this script runs on your machine; pass --local explicitly or use --dry-run")
		}
		if err := item.CheckSupport("oracle", runtime.GOOS, runtime.GOARCH); err != nil {
			return a.fail(err.Error())
		}
		if _, exists := item.Inputs["yes"]; exists {
			values["yes"] = *yes
		}
		data, err := recipe.RenderLocalScript(item, options)
		if err != nil {
			return a.fail("prepare local script: " + err.Error())
		}
		if *dryRun {
			return a.emit(map[string]any{"recipe": item.Name, "execution": "local", "dry_run": true}, nil)
		}
		if !*yes && !isTerminal(a.in) {
			return a.fail("local script execution requires --yes in noninteractive mode")
		}
		if !*yes && !a.confirm("execute local script "+item.Name) {
			return 1
		}
		if _, exists := item.Inputs["yes"]; exists {
			values["yes"] = true
			data, err = recipe.RenderLocalScript(item, options)
			if err != nil {
				return a.fail(err.Error())
			}
		}
		binary, err := os.Executable()
		if err != nil {
			return a.fail(err.Error())
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-s")
		cmd.Stdin = strings.NewReader(data)
		cmd.Stdout, cmd.Stderr = a.out, a.err
		cmd.Env = append(os.Environ(), "INFINITY_IAC_BINARY="+binary)
		if err := cmd.Run(); err != nil {
			return a.fail("local script execution failed: " + err.Error())
		}
		return 0
	}
	if *local {
		return a.fail("this recipe requires an instance; --local cannot run guest bootstrap on your machine")
	}
	bootstrap, err := recipe.RenderCloudInitWithInputs(item, options)
	if err != nil {
		return a.fail("prepare recipe: " + err.Error())
	}
	if *dryRun {
		return a.emit(map[string]any{"recipe": item.Name, "execution": "instance", "dry_run": true, "cpu": item.Resources.CPU, "ram_gb": item.Resources.RAMGB, "storage_gb": item.Resources.StorageGB}, nil)
	}
	profile, ok := a.cfg.DefaultProfile()
	if *access != "" {
		profile, ok = a.cfg.Profile(*access)
	}
	if !ok {
		return a.fail("no access profile configured; run setup oracle")
	}
	architecture := "amd64"
	if strings.HasPrefix(profile.Shape, "VM.Standard.A1.") || strings.HasPrefix(profile.Shape, "VM.Standard.A2.") {
		architecture = "arm64"
	}
	if len(item.Supports.Architectures) > 0 {
		supported := false
		for _, allowed := range item.Supports.Architectures {
			if allowed == architecture {
				supported = true
			}
		}
		if !supported {
			return a.fail(fmt.Sprintf("recipe %q does not support architecture %q", item.Name, architecture))
		}
	}
	if len(item.Supports.Providers) > 0 {
		supported := false
		for _, allowed := range item.Supports.Providers {
			if allowed == profile.Provider || (allowed == "oracle" || allowed == "oci") && (profile.Provider == "oracle" || profile.Provider == "oci") {
				supported = true
			}
		}
		if !supported {
			return a.fail("recipe does not support the selected provider")
		}
	}
	if len(item.Supports.OS) > 0 {
		backend, err := a.providerForProfile(profile)
		if err != nil {
			return a.fail(err.Error())
		}
		reader, ok := backend.(interface {
			OperatingSystem(context.Context) (string, error)
		})
		if !ok {
			return a.fail("provider cannot verify the recipe operating system")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		osName, err := reader.OperatingSystem(ctx)
		cancel()
		if err != nil {
			return a.fail("inspect configured image: " + err.Error())
		}
		if err := item.CheckSupport(profile.Provider, osName, architecture); err != nil {
			return a.fail(err.Error())
		}
	}
	if *name == "" {
		*name = target
	}
	cpu, ram, storage := recipeResources(item)
	createArgs := []string{"--name", *name, "--access", profile.Name, "--cpu", strconv.Itoa(cpu), "--ram", strconv.Itoa(ram) + "GB", "--storage-size", strconv.Itoa(storage) + "GB", "--port", strconv.Itoa(*port), "--yes=" + strconv.FormatBool(*yes), "--output", *mode}
	if *startup != "" {
		createArgs = append(createArgs, "--startup-script", *startup)
	}
	return a.createServer(createArgs, "run-"+target, bootstrap)
}

func findRecipe(name string) (recipe.Recipe, bool) {
	items, _, err := loadRecipes("")
	if err != nil {
		return recipe.Recipe{}, false
	}
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return recipe.Recipe{}, false
}

func recipePort(item recipe.Recipe) int {
	if in, ok := item.Inputs["port"]; ok {
		if value, ok := in.Default.(int); ok {
			return value
		}
		if value, ok := in.Default.(int64); ok {
			return int(value)
		}
		if value, ok := in.Default.(float64); ok {
			return int(value)
		}
	}
	if len(item.Resources.Ingress) > 0 {
		return item.Resources.Ingress[0].Port
	}
	return 0
}

func recipeResources(item recipe.Recipe) (cpu, ram, storage int) {
	cpu, ram, storage = item.Resources.CPU, item.Resources.RAMGB, item.Resources.StorageGB
	if cpu < 1 {
		cpu = 1
	}
	if ram < 1 {
		ram = 1
	}
	if storage < 1 {
		storage = 20
	}
	return cpu, ram, storage
}

func recipeCloudInit(item recipe.Recipe, port int) (string, error) {
	return recipe.RenderCloudInit(item, port)
}

func (a *App) instance(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: instance get|start|stop|restart|delete ID [--access NAME] [--wait] [--timeout 10m] [--yes] [--output json]")
		return 0
	}
	action := args[0]
	if action != "get" && action != "start" && action != "stop" && action != "restart" && action != "delete" {
		return a.fail("unknown instance action " + action)
	}
	fs := flag.NewFlagSet("instance "+action, flag.ContinueOnError)
	fs.SetOutput(a.err)
	yes := fs.Bool("yes", false, "confirm mutation")
	access := fs.String("access", "", "access profile")
	wait := fs.Bool("wait", false, "wait for the operation to finish")
	timeout := fs.Duration("timeout", 10*time.Minute, "operation deadline")
	mode := fs.String("output", "table", "table or json")
	parseArgs, err := flagsBeforePositionals(fs, args[1:])
	if err != nil {
		return a.fail(err.Error())
	}
	if err := fs.Parse(parseArgs); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		return a.fail("instance ID or name is required")
	}
	if *mode != "table" && *mode != "json" {
		return a.fail("output must be table or json")
	}
	if *timeout <= 0 || *timeout > time.Hour {
		return a.fail("timeout must be greater than zero and at most one hour")
	}
	profile, ok := a.cfg.DefaultProfile()
	if *access != "" {
		profile, ok = a.cfg.Profile(*access)
	}
	if !ok {
		return a.fail("no access profile configured; run setup oracle")
	}
	p, err := a.providerForProfile(profile)
	if err != nil {
		return a.fail(err.Error())
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	items, err := p.ListInstances(ctx, provider.Filter{Access: profile.Name})
	if err != nil {
		return a.fail(err.Error())
	}
	id := ""
	for _, item := range items {
		if item.ID == fs.Arg(0) || item.Name == fs.Arg(0) {
			if id != "" && id != item.ID {
				return a.fail("instance name is ambiguous; use an exact ID")
			}
			id = item.ID
		}
	}
	if id == "" {
		return a.fail(fmt.Sprintf("managed instance %q not found", fs.Arg(0)))
	}
	if action != "get" {
		if !*yes && !isTerminal(a.in) {
			return a.fail("instance mutation requires --yes in noninteractive mode")
		}
		if !*yes && !a.confirm(action+" instance "+id) {
			return 1
		}
	}
	if action == "delete" {
		if *wait {
			if waiter, ok := p.(interface {
				DeleteInstanceAndWait(context.Context, string) error
			}); ok {
				err = waiter.DeleteInstanceAndWait(ctx, id)
			} else {
				return a.fail("provider does not support confirmed deletion")
			}
		} else {
			err = p.DeleteInstance(ctx, id)
		}
		if err != nil {
			return a.fail(err.Error())
		}
		status := "terminating"
		if *wait {
			status = domain.StatusDeleted
		}
		if *mode == "json" {
			return a.emit(map[string]any{"id": id, "status": status, "confirmed": *wait}, nil)
		}
		fmt.Fprintln(a.out, status, id)
		return 0
	}
	var item domain.Instance
	switch action {
	case "get":
		if reader, ok := p.(interface {
			GetInstance(context.Context, string) (domain.Instance, error)
		}); ok {
			item, err = reader.GetInstance(ctx, id)
		} else {
			return a.fail("provider does not support instance inspection")
		}
	case "start":
		if starter, ok := p.(interface {
			StartInstance(context.Context, string) (domain.Instance, error)
		}); ok {
			item, err = starter.StartInstance(ctx, id)
		} else {
			return a.fail("provider does not support starting instances")
		}
	case "stop":
		item, err = p.StopInstance(ctx, id)
	case "restart":
		item, err = p.RestartInstance(ctx, id)
	}
	if err != nil {
		return a.fail(err.Error())
	}
	if *wait && action != "get" {
		status := domain.StatusRunning
		if action == "stop" {
			status = domain.StatusStopped
		}
		if waiter, ok := p.(interface {
			WaitInstance(context.Context, string, string) (domain.Instance, error)
		}); ok {
			item, err = waiter.WaitInstance(ctx, id, status)
		} else {
			return a.fail("provider does not support instance waiting")
		}
		if err != nil {
			return a.fail(err.Error())
		}
	}
	if *mode == "json" {
		return a.emit(item, nil)
	}
	fmt.Fprintf(a.out, "%s %s (%s)\n", action, item.Name, item.Status)
	return 0
}
func (a *App) resolveInstance(p provider.Provider, value string) (string, error) {
	items, err := p.ListInstances(context.Background(), provider.Filter{})
	if err != nil {
		return "", err
	}
	for _, i := range items {
		if i.ID == value || i.Name == value {
			return i.ID, nil
		}
	}
	return "", fmt.Errorf("instance %q not found", value)
}
func (a *App) doctor(args []string) int {
	mode, err := outputMode(args)
	if err != nil {
		return a.fail(err.Error())
	}
	providerName := ""
	if profile, ok := a.cfg.DefaultProfile(); ok {
		providerName = profile.Provider
	}
	result := map[string]any{"version": Version, "config": a.configPath, "profiles": len(a.cfg.Profiles), "provider": providerName}
	if mode == "json" {
		return a.emit(result, nil)
	}
	fmt.Fprintf(a.out, "version: %s\nconfig: %s\nprofiles: %d\nprovider: %s\n", result["version"], result["config"], result["profiles"], result["provider"])
	return 0
}

func (a *App) plan(args []string, apply bool) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(a.err)
	file := fs.String("file", "", "JSON manifest file")
	hash := fs.String("hash", "", "approved plan hash (required by apply)")
	yes := fs.Bool("yes", false, "confirm apply")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *file == "" {
		return a.fail("--file is required")
	}
	if fs.NArg() != 0 || *mode != "table" && *mode != "json" {
		return a.fail("plan/apply accepts flags only and --output table|json")
	}
	b, err := os.ReadFile(*file)
	if err != nil {
		return a.fail(err.Error())
	}
	var m domain.Manifest
	if strings.HasSuffix(strings.ToLower(*file), ".yaml") || strings.HasSuffix(strings.ToLower(*file), ".yml") {
		if err := yaml.Unmarshal(b, &m); err != nil {
			return a.fail("decode manifest: " + err.Error())
		}
	} else if err := json.Unmarshal(b, &m); err != nil {
		return a.fail("decode manifest: " + err.Error())
	}
	profile, ok := a.cfg.DefaultProfile()
	if !ok {
		return a.fail("no access profile configured; run setup oracle")
	}
	for i := range m.Servers {
		m.Servers[i] = m.Servers[i].ResolveDefaults(profile.Provider, profile.Name, profile.Region)
		if m.Servers[i].Access != profile.Name || m.Servers[i].Region != profile.Region || m.Servers[i].Provider != profile.Provider {
			return a.fail("a plan must target its selected access profile, region, and provider")
		}
		if m.Servers[i].StartupScript != "" {
			path := m.Servers[i].StartupScript
			if !filepath.IsAbs(path) {
				path = filepath.Join(filepath.Dir(*file), path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return a.fail("read plan startup script: " + err.Error())
			}
			m.Servers[i].UserData = string(data)
		}
	}
	backend, err := a.provider()
	if err != nil {
		return a.fail(err.Error())
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	existing, err := backend.ListInstances(ctx, provider.Filter{Access: profile.Name})
	if err != nil {
		return a.fail(err.Error())
	}
	plan, err := domain.NewPlan(m, existing, time.Now().UTC())
	if err != nil {
		return a.fail(err.Error())
	}
	if !apply {
		if *mode == "json" {
			return a.emit(plan, nil)
		}
		fmt.Fprintf(a.out, "plan %s: %d action(s)\n", plan.ID, len(plan.Actions))
		for _, action := range plan.Actions {
			fmt.Fprintf(a.out, "  %s %s\n", action.Type, action.Resource)
		}
		return 0
	}
	if *hash == "" {
		return a.fail("apply requires --hash from a reviewed plan")
	}
	if *hash != plan.Hash {
		return a.fail("plan hash does not match the reviewed manifest")
	}
	if len(plan.Actions) == 0 {
		return 0
	}
	if !*yes && !isTerminal(a.in) {
		return a.fail("apply requires --yes in noninteractive mode")
	}
	if !*yes && !a.confirm("apply plan "+plan.ID) {
		return 1
	}
	results := []domain.Instance{}
	for _, action := range plan.Actions {
		if action.Type != "create" || action.Spec == nil {
			continue
		}
		i, err := backend.CreateInstance(ctx, provider.CreateRequest{Spec: *action.Spec, IdempotencyKey: "plan-" + plan.Hash[:48] + "-" + strconv.Itoa(len(results)), UserData: action.Spec.UserData})
		if err != nil {
			return a.fail(err.Error())
		}
		results = append(results, i)
	}
	if *mode == "json" {
		return a.emit(map[string]any{"plan": plan, "created": results}, nil)
	}
	fmt.Fprintf(a.out, "applied %d action(s)\n", len(results))
	return 0
}
func (a *App) install(args []string) int {
	if len(args) == 0 || args[0] != "skills" {
		fmt.Fprintln(a.out, "usage: install skills [--dry-run]")
		return 0
	}
	fs := flag.NewFlagSet("install skills", flag.ContinueOnError)
	fs.SetOutput(a.err)
	dry := fs.Bool("dry-run", false, "show changes only")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return a.fail(err.Error())
	}
	path := filepath.Join(base, "infinity-iac", "skills", "infinity-iac.md")
	if *dry {
		fmt.Fprintln(a.out, path)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return a.fail(err.Error())
	}
	content := "# Infinity-IaC\n\nUse `infinity-iac plan` before billable changes. Use `--output json` for structured results.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return a.fail(err.Error())
	}
	fmt.Fprintln(a.out, path)
	return 0
}

func (a *App) emit(value any, table func()) int {
	if table != nil {
		table()
		return 0
	}
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		fmt.Fprintf(a.err, "error: %v\n", err)
		return 1
	}
	return 0
}
func (a *App) fail(message string) int { fmt.Fprintln(a.err, "error:", message); return 1 }
func (a *App) confirm(action string) bool {
	fmt.Fprintf(a.out, "confirm %s? [y/N] ", action)
	line, _ := bufio.NewReader(a.in).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes")
}
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && (func() bool { info, err := f.Stat(); return err == nil && info.Mode()&os.ModeCharDevice != 0 })()
}
func outputMode(args []string) (string, error) {
	mode := "table"
	for i, arg := range args {
		if arg == "--output" && i+1 < len(args) {
			mode = args[i+1]
		}
		if strings.HasPrefix(arg, "--output=") {
			mode = strings.TrimPrefix(arg, "--output=")
		}
	}
	if mode != "table" && mode != "json" {
		return "", errors.New("output must be table or json")
	}
	return mode, nil
}
func sizeGB(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	s := strings.ToUpper(strings.TrimSpace(value))
	s = strings.TrimSuffix(s, "GB")
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid size %q", value)
	}
	return n, nil
}
