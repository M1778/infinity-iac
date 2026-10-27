package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/config"
	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/M1778/infinity-iac/internal/provider"
)

type App struct {
	in         io.Reader
	out, err   io.Writer
	cfg        config.File
	configPath string
	p          *provider.FileProvider
}

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
	a.p = provider.NewFileProvider(cfg.EffectiveStatePath(path))
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		a.help()
		return 0
	}
	switch args[0] {
	case "setup":
		return a.setup(args[1:])
	case "list":
		return a.list(args[1:])
	case "create":
		return a.create(args[1:])
	case "run":
		return a.run(args[1:])
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
	case "completion":
		return a.completion(args[1:])
	default:
		fmt.Fprintf(a.err, "error: unknown command %q\n", args[0])
		return 2
	}
}
func (a *App) help() {
	fmt.Fprintln(a.out, "Infinity-IaC: infrastructure lifecycle CLI")
	fmt.Fprintln(a.out, "commands: setup, list, create, run, instance, plan, apply, install, completion, doctor")
	fmt.Fprintln(a.out, "examples:")
	fmt.Fprintln(a.out, "  infinity-iac setup oracle --name default --profile DEFAULT --region us-phoenix-1")
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
		fmt.Fprintln(a.out, "usage: setup oracle [--name NAME] [--profile PROFILE] [--account ACCOUNT] [--region REGION]")
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
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *name == "" {
		*name = "default"
	}
	if *region == "" {
		*region = os.Getenv("OCI_REGION")
	}
	if *region == "" {
		*region = "local"
	}
	if *profile == "" {
		*profile = "DEFAULT"
	}
	p := config.NewProfile(*name, "oracle", "oci-profile:"+*profile, *account, *region)
	a.cfg.UpsertProfile(p)
	if err := config.Save(a.configPath, a.cfg); err != nil {
		return a.fail("save config: " + err.Error())
	}
	return a.emit(map[string]any{"profile": p, "config": a.configPath, "validation": "credential source recorded; live provider validation is unavailable in the local adapter"}, func() { fmt.Fprintf(a.out, "configured access %q for oracle (%s)\n", p.Name, p.CredentialSource) })
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
	default:
		return a.fail("unknown list target " + args[0])
	}
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
	mode, err := outputMode(args)
	if err != nil {
		return a.fail(err.Error())
	}
	items, err := a.p.ListInstances(context.Background(), provider.Filter{})
	if err != nil {
		return a.fail(err.Error())
	}
	if mode == "json" {
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
	if args[0] != "server" {
		return a.fail("unknown create target " + args[0])
	}
	return a.createServer(args[1:], "server")
}
func (a *App) createServer(args []string, kind string) int {
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
	if *amount < 1 {
		return a.fail("amount must be at least 1")
	}
	profile, ok := a.cfg.DefaultProfile()
	if !ok && (*providerName == "" || *access == "") {
		return a.fail("no access profile configured; run setup oracle or pass --provider and --access")
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
	for n := 0; n < *amount; n++ {
		nme := base
		if *amount > 1 {
			nme = fmt.Sprintf("%s-%d", base, n+1)
		}
		spec := domain.ServerSpec{Name: nme, OS: *osName, CPU: *cpu, RAMGB: rams, StorageGB: storages, Provider: *providerName, Access: *access, Region: *region, NetworkingScript: *networking, StartupScript: *startup, Port: *port}.ResolveDefaults(*providerName, *access, *region)
		if err := spec.Validate(); err != nil {
			return a.fail(err.Error())
		}
		i, err := a.p.CreateInstance(context.Background(), provider.CreateRequest{Spec: spec, IdempotencyKey: "create-" + nme})
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
		fmt.Fprintln(a.out, "usage: run --app NAME [--specs auto] [--port N] [--yes]")
		return 0
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(a.err)
	app := fs.String("app", "", "application name")
	script := fs.String("script", "", "recipe name")
	specs := fs.String("specs", "auto", "specification preset")
	port := fs.Int("port", 0, "application port")
	name := fs.String("name", "", "instance name")
	yes := fs.Bool("yes", false, "confirm")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *app == "" && *script == "" {
		return a.fail("run requires --app or --script")
	}
	target := *app
	if target == "" {
		target = *script
	}
	if *name == "" {
		*name = target
	}
	if *specs != "" && *specs != "auto" {
		return a.fail("only specs=auto is supported in this release")
	}
	_ = *port
	return a.createServer([]string{"--name", *name, "--port", strconv.Itoa(*port), "--yes=" + strconv.FormatBool(*yes), "--output", *mode}, "run-"+target)
}

func (a *App) instance(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: instance stop|restart|delete ID [--yes] [--output json]")
		return 0
	}
	action := args[0]
	if action != "stop" && action != "restart" && action != "delete" {
		return a.fail("unknown instance action " + action)
	}
	// Normalize positional instance ID and flags before parsing because Go's
	// flag package stops parsing flags after the first positional argument.
	var positional string
	parseArgs := make([]string, 0, len(args[1:]))
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			parseArgs = append(parseArgs, arg)
		} else if positional == "" {
			positional = arg
		} else {
			parseArgs = append(parseArgs, arg)
		}
	}
	if positional != "" {
		parseArgs = append(parseArgs, positional)
	}
	fs := flag.NewFlagSet("instance "+action, flag.ContinueOnError)
	fs.SetOutput(a.err)
	yes := fs.Bool("yes", false, "confirm deletion")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(parseArgs); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		return a.fail("instance ID or name is required")
	}
	id, err := a.resolveInstance(fs.Arg(0))
	if err != nil {
		return a.fail(err.Error())
	}
	if action == "delete" {
		if !*yes && !isTerminal(a.in) {
			return a.fail("deletion requires --yes in noninteractive mode")
		}
		if !*yes && !a.confirm("delete instance "+id) {
			return 1
		}
		err = a.p.DeleteInstance(context.Background(), id)
		if err != nil {
			return a.fail(err.Error())
		}
		if *mode == "json" {
			return a.emit(map[string]any{"id": id, "status": domain.StatusDeleted}, nil)
		}
		fmt.Fprintln(a.out, "deleted", id)
		return 0
	}
	var item domain.Instance
	if action == "stop" {
		item, err = a.p.StopInstance(context.Background(), id)
	} else {
		item, err = a.p.RestartInstance(context.Background(), id)
	}
	if err != nil {
		return a.fail(err.Error())
	}
	if *mode == "json" {
		return a.emit(item, nil)
	}
	fmt.Fprintf(a.out, "%s %s (%s)\n", action, item.Name, item.Status)
	return 0
}
func (a *App) resolveInstance(value string) (string, error) {
	items, err := a.p.ListInstances(context.Background(), provider.Filter{})
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
	result := map[string]any{"config": a.configPath, "profiles": len(a.cfg.Profiles), "provider": "local", "state": a.cfg.EffectiveStatePath(a.configPath), "oracle_adapter": "not configured in local vertical slice"}
	if mode == "json" {
		return a.emit(result, nil)
	}
	fmt.Fprintf(a.out, "config: %s\nprofiles: %d\nstate: %s\nprovider: local\n", result["config"], result["profiles"], result["state"])
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
	if profile, ok := a.cfg.DefaultProfile(); ok {
		for i := range m.Servers {
			m.Servers[i] = m.Servers[i].ResolveDefaults(profile.Provider, profile.Name, profile.Region)
		}
	}
	existing, err := a.p.ListInstances(context.Background(), provider.Filter{})
	if err != nil {
		return a.fail(err.Error())
	}
	p, err := domain.NewPlan(m, existing, time.Now().UTC())
	if err != nil {
		return a.fail(err.Error())
	}
	if !apply {
		if *mode == "json" {
			return a.emit(p, nil)
		}
		fmt.Fprintf(a.out, "plan %s: %d action(s)\n", p.ID, len(p.Actions))
		for _, action := range p.Actions {
			fmt.Fprintf(a.out, "  %s %s\n", action.Type, action.Resource)
		}
		return 0
	}
	if *hash == "" {
		return a.fail("apply requires --hash from a reviewed plan")
	}
	if *hash != p.Hash {
		return a.fail("plan hash does not match the reviewed manifest")
	}
	if len(p.Actions) == 0 {
		return 0
	}
	if !*yes && !isTerminal(a.in) {
		return a.fail("apply requires --yes in noninteractive mode")
	}
	if !*yes && !a.confirm("apply plan "+p.ID) {
		return 1
	}
	results := []domain.Instance{}
	for _, action := range p.Actions {
		if action.Type != "create" || action.Spec == nil {
			continue
		}
		i, err := a.p.CreateInstance(context.Background(), provider.CreateRequest{Spec: *action.Spec, IdempotencyKey: "plan-" + p.ID + "-" + action.ID})
		if err != nil {
			return a.fail(err.Error())
		}
		results = append(results, i)
	}
	if *mode == "json" {
		return a.emit(map[string]any{"plan": p, "created": results}, nil)
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
