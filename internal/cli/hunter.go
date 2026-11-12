package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/M1778/infinity-iac/internal/provider"
	"github.com/gofrs/flock"
	"github.com/oracle/oci-go-sdk/v65/common"
)

type hunterOptions struct {
	Profile   domain.AccessProfile
	Spec      domain.ServerSpec
	Domains   []string
	Interval  time.Duration
	Attempts  int
	StateFile string
}

type hunterState struct {
	Schema      int               `json:"schema"`
	Fingerprint string            `json:"fingerprint"`
	Tokens      map[string]string `json:"tokens"`
	Domain      int               `json:"domain"`
	Attempts    int               `json:"attempts"`
	Pending     bool              `json:"pending"`
	InstanceID  string            `json:"instance_id,omitempty"`
}

func (a *App) ocHunter(args []string) int {
	fs := flag.NewFlagSet("oc-hunter", flag.ContinueOnError)
	fs.SetOutput(a.err)
	access := fs.String("access", "", "configured OCI access profile")
	name := fs.String("name", "oc-hunter", "unique managed instance name")
	domains := fs.String("availability-domains", "", "comma-separated exact availability domain names; defaults to the profile domain")
	shape := fs.String("shape", "", "VM.Standard.A1.Flex or VM.Standard.E2.1.Micro; defaults to the profile shape")
	cpu := fs.Int("cpu", 2, "requested vCPUs")
	ram := fs.String("ram", "12GB", "requested memory")
	storage := fs.String("storage-size", "50GB", "boot volume allocation")
	interval := fs.Duration("interval", time.Minute, "minimum time between attempts, at least 30s")
	attempts := fs.Int("attempts", 20, "maximum launch attempts, between 1 and 100")
	timeout := fs.Duration("timeout", 30*time.Minute, "deadline, at most 8h")
	stateFile := fs.String("state-file", "", "private operation journal; retain it after an ambiguous launch")
	yes := fs.Bool("yes", false, "confirm potentially billable instance creation")
	dryRun := fs.Bool("dry-run", false, "validate settings without cloud calls or journal writes")
	mode := fs.String("output", "table", "table or json")
	if len(args) == 1 && args[0] == "help" {
		fs.PrintDefaults()
		return 0
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || *mode != "table" && *mode != "json" {
		return a.fail("oc-hunter accepts flags only and --output table|json")
	}
	if *interval < 30*time.Second || *interval > 30*time.Minute || *attempts < 1 || *attempts > 100 || *timeout <= 0 || *timeout > 8*time.Hour {
		return a.fail("interval must be 30s–30m, attempts 1–100, and timeout greater than zero and at most 8h")
	}
	profile, ok := a.cfg.DefaultProfile()
	if *access != "" {
		profile, ok = a.cfg.Profile(*access)
	}
	if !ok {
		return a.fail("no access profile configured; run setup oracle")
	}
	if *shape != "" {
		profile.Shape = *shape
	}
	if profile.Shape != "VM.Standard.A1.Flex" && profile.Shape != "VM.Standard.E2.1.Micro" {
		return a.fail("oc-hunter supports VM.Standard.A1.Flex and VM.Standard.E2.1.Micro only; no shape guarantees zero cost")
	}
	memory, err := sizeGB(*ram, 12)
	if err != nil {
		return a.fail(err.Error())
	}
	volume, err := sizeGB(*storage, 50)
	if err != nil {
		return a.fail(err.Error())
	}
	if volume < 50 {
		return a.fail("boot storage must be at least 50GB")
	}
	if profile.Shape == "VM.Standard.E2.1.Micro" {
		if *cpu != 1 || memory != 1 {
			return a.fail("the fixed Micro shape requires --cpu 1 --ram 1GB")
		}
	}
	selected := []string{profile.AvailabilityDomain}
	if *domains != "" {
		selected = strings.Split(*domains, ",")
		seen := make(map[string]bool)
		for i, domainName := range selected {
			selected[i] = strings.TrimSpace(domainName)
			if selected[i] == "" || seen[selected[i]] {
				return a.fail("availability domains must be nonempty and unique")
			}
			seen[selected[i]] = true
		}
	}
	if len(selected) > 3 || selected[0] == "" {
		return a.fail("select one to three exact availability domain names")
	}
	spec := domain.ServerSpec{Name: *name, Provider: profile.Provider, Access: profile.Name, Region: profile.Region, OS: "configured-image", CPU: *cpu, RAMGB: memory, StorageGB: volume}
	if err := spec.Validate(); err != nil {
		return a.fail(err.Error())
	}
	if profile.Provider != "oracle" && profile.Provider != "oci" {
		return a.fail("oc-hunter requires an OCI profile")
	}
	if *stateFile == "" {
		*stateFile = filepath.Join(filepath.Dir(a.configPath), "operations", profile.Name+"-"+*name+".json")
	}
	if *dryRun {
		return a.emit(map[string]any{"dry_run": true, "name": spec.Name, "access": profile.Name, "shape": profile.Shape, "cpu": spec.CPU, "ram_gb": memory, "storage_gb": volume, "availability_domains": selected, "attempts": *attempts, "interval": interval.String(), "timeout": timeout.String()}, nil)
	}
	if !*yes && !isTerminal(a.in) {
		return a.fail("oc-hunter requires --yes in noninteractive mode")
	}
	if !*yes && !a.confirm("create one instance with oc-hunter; free-tier eligibility and billing must be checked separately") {
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	options := hunterOptions{Profile: profile, Spec: spec, Domains: selected, Interval: *interval, Attempts: *attempts, StateFile: *stateFile}
	factory := func(domainName string) (provider.Provider, error) {
		p := profile
		p.AvailabilityDomain = domainName
		backend, err := a.providerForProfile(p)
		if err != nil {
			return nil, err
		}
		if validator, ok := backend.(interface{ ValidateAccess(context.Context) error }); ok {
			if err := validator.ValidateAccess(ctx); err != nil {
				return nil, err
			}
		}
		return backend, nil
	}
	item, err := runHunter(ctx, options, factory, hunterPause, a.err)
	if err != nil {
		return a.fail(err.Error())
	}
	if *mode == "json" {
		return a.emit(item, nil)
	}
	fmt.Fprintf(a.out, "created %s (%s); retain %s for operation reconciliation\n", item.Name, item.ID, *stateFile)
	return 0
}

func hunterPause(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func saveHunterState(path string, state hunterState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".oc-hunter-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func runHunter(ctx context.Context, options hunterOptions, factory func(string) (provider.Provider, error), pause func(context.Context, time.Duration) error, log io.Writer) (domain.Instance, error) {
	if options.Attempts < 1 || options.Attempts > 100 || options.Interval < 30*time.Second || options.Interval > 30*time.Minute || len(options.Domains) == 0 || len(options.Domains) > 3 {
		return domain.Instance{}, errors.New("invalid bounded hunter settings")
	}
	if err := options.Spec.Validate(); err != nil {
		return domain.Instance{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.Instance{}, err
	}
	if err := os.MkdirAll(filepath.Dir(options.StateFile), 0o700); err != nil {
		return domain.Instance{}, err
	}
	for _, path := range []string{options.StateFile, options.StateFile + ".lock"} {
		info, err := os.Lstat(path)
		if err == nil && !info.Mode().IsRegular() {
			return domain.Instance{}, errors.New("hunter journal and lock must be regular files, not symlinks")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return domain.Instance{}, err
		}
	}
	lock := flock.New(options.StateFile + ".lock")
	locked, err := lock.TryLock()
	if err != nil {
		return domain.Instance{}, err
	}
	if !locked {
		return domain.Instance{}, errors.New("another hunter is using this operation journal")
	}
	defer lock.Unlock()
	if err := os.Chmod(options.StateFile+".lock", 0o600); err != nil {
		return domain.Instance{}, err
	}
	identity, err := json.Marshal(struct {
		Profile domain.AccessProfile
		Spec    domain.ServerSpec
		Domains []string
	}{options.Profile, options.Spec, options.Domains})
	if err != nil {
		return domain.Instance{}, err
	}
	hash := sha256.Sum256(identity)
	fingerprint := hex.EncodeToString(hash[:])
	state := hunterState{Schema: 1, Fingerprint: fingerprint, Tokens: make(map[string]string)}
	data, err := os.ReadFile(options.StateFile)
	if err == nil {
		if len(data) > 1<<20 || json.Unmarshal(data, &state) != nil || state.Schema != 1 || state.Fingerprint != fingerprint || len(state.Tokens) != len(options.Domains) || state.Domain < 0 || state.Domain >= len(options.Domains) || state.Attempts < 0 {
			return domain.Instance{}, errors.New("hunter journal is invalid or belongs to different launch settings")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.Instance{}, err
	} else {
		for _, domainName := range options.Domains {
			token := make([]byte, 16)
			if _, err := rand.Read(token); err != nil {
				return domain.Instance{}, err
			}
			state.Tokens[domainName] = "hunter-" + hex.EncodeToString(token)
		}
		if err := saveHunterState(options.StateFile, state); err != nil {
			return domain.Instance{}, err
		}
	}
	clients := make(map[string]provider.Provider)
	for _, domainName := range options.Domains {
		if state.Tokens[domainName] == "" {
			return domain.Instance{}, errors.New("hunter journal is missing an operation token")
		}
		client, err := factory(domainName)
		if err != nil {
			return domain.Instance{}, err
		}
		clients[domainName] = client
	}
	reconcile := func() (*domain.Instance, error) {
		client := clients[options.Domains[state.Domain]]
		items, err := client.ListInstances(ctx, provider.Filter{Access: options.Profile.Name})
		if err != nil {
			return nil, err
		}
		var result *domain.Instance
		for _, item := range items {
			if item.Name != options.Spec.Name || item.Status == domain.StatusDeleted {
				continue
			}
			matched := false
			for _, token := range state.Tokens {
				if item.Metadata["infinity-iac-operation"] == token {
					matched = true
				}
			}
			if !matched || item.Owner != "infinity-iac" || item.Access != options.Profile.Name || result != nil {
				return nil, errors.New("instance name conflicts with a foreign, legacy, or duplicate operation")
			}
			copy := item
			result = &copy
		}
		return result, nil
	}
	if found, err := reconcile(); err != nil {
		return domain.Instance{}, err
	} else if found != nil {
		state.Pending, state.InstanceID = false, found.ID
		if err := saveHunterState(options.StateFile, state); err != nil {
			return *found, err
		}
		return *found, nil
	}
	if state.Pending || state.InstanceID != "" {
		return domain.Instance{}, errors.New("prior launch outcome is uncertain or no longer visible; inspect the recorded operation before starting another")
	}
	for state.Attempts < options.Attempts {
		if err := ctx.Err(); err != nil {
			return domain.Instance{}, err
		}
		domainName := options.Domains[state.Domain]
		token := state.Tokens[domainName]
		state.Pending = true
		state.Attempts++
		if err := saveHunterState(options.StateFile, state); err != nil {
			return domain.Instance{}, err
		}
		fmt.Fprintf(log, "oc-hunter attempt %d/%d in %s\n", state.Attempts, options.Attempts, domainName)
		item, launchErr := clients[domainName].CreateInstance(ctx, provider.CreateRequest{Spec: options.Spec, IdempotencyKey: token})
		if launchErr == nil && item.ID != "" {
			if item.Owner != "infinity-iac" || item.Access != options.Profile.Name || item.Name != options.Spec.Name || item.Metadata["infinity-iac-operation"] != token {
				return domain.Instance{}, errors.New("launch returned an unexpected identity; journal retained for inspection")
			}
			state.Pending, state.InstanceID = false, item.ID
			if err := saveHunterState(options.StateFile, state); err != nil {
				return item, err
			}
			return item, nil
		}
		found, reconcileErr := reconcile()
		if reconcileErr != nil {
			return domain.Instance{}, errors.New("launch reconciliation failed; hunter stopped and retained the operation journal")
		}
		if found != nil {
			state.Pending, state.InstanceID = false, found.ID
			if err := saveHunterState(options.StateFile, state); err != nil {
				return *found, err
			}
			return *found, nil
		}
		var service common.ServiceError
		if !errors.As(launchErr, &service) {
			return domain.Instance{}, errors.New("launch outcome is ambiguous; hunter stopped and retained the exact operation journal")
		}
		message := strings.ToLower(service.GetMessage())
		capacity := service.GetHTTPStatusCode() >= 500 && (strings.Contains(message, "out of host capacity") || strings.Contains(message, "out of capacity for shape"))
		throttled := service.GetHTTPStatusCode() == 429
		if !capacity && !throttled {
			return domain.Instance{}, fmt.Errorf("hunter stopped for OCI %s (HTTP %d); inspect the journal before retrying", service.GetCode(), service.GetHTTPStatusCode())
		}
		state.Pending = false
		if capacity {
			state.Domain = (state.Domain + 1) % len(options.Domains)
		}
		if err := saveHunterState(options.StateFile, state); err != nil {
			return domain.Instance{}, err
		}
		if state.Attempts == options.Attempts {
			break
		}
		maxDelay := 15 * time.Minute
		if options.Interval > maxDelay {
			maxDelay = options.Interval
		}
		delay := options.Interval
		for step := 1; step < state.Attempts && delay < maxDelay; step++ {
			delay *= 2
		}
		if delay > maxDelay {
			delay = maxDelay
		}
		var retry interface{ RetryAfter() time.Duration }
		if errors.As(launchErr, &retry) && retry.RetryAfter() > delay {
			delay = retry.RetryAfter()
		}
		if err := pause(ctx, delay); err != nil {
			return domain.Instance{}, err
		}
	}
	return domain.Instance{}, errors.New("hunter reached its attempt limit without a confirmed allocation")
}
