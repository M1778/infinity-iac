package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/M1778/infinity-iac/internal/provider"
)

type hunterServiceError struct {
	status  int
	code    string
	message string
}

func (e hunterServiceError) Error() string           { return e.code }
func (e hunterServiceError) GetHTTPStatusCode() int  { return e.status }
func (e hunterServiceError) GetCode() string         { return e.code }
func (e hunterServiceError) GetMessage() string      { return e.message }
func (e hunterServiceError) GetOpcRequestID() string { return "test-request" }

type hunterTestProvider struct {
	*lifecycleProvider
	create func(provider.CreateRequest) (domain.Instance, error)
	calls  int
}

func (p *hunterTestProvider) CreateInstance(_ context.Context, request provider.CreateRequest) (domain.Instance, error) {
	p.calls++
	return p.create(request)
}

func testHunterOptions(t *testing.T) hunterOptions {
	t.Helper()
	return hunterOptions{Profile: domain.AccessProfile{Name: "test", Provider: "oracle", Region: "us-phoenix-1", Shape: "VM.Standard.A1.Flex"}, Spec: domain.ServerSpec{Name: "hunt-test", Provider: "oracle", Access: "test", Region: "us-phoenix-1", OS: "configured-image", CPU: 2, RAMGB: 12, StorageGB: 50}, Domains: []string{"AD-1", "AD-2"}, Interval: time.Minute, Attempts: 3, StateFile: filepath.Join(t.TempDir(), "operation.json")}
}

func allocatedHunterInstance(request provider.CreateRequest) domain.Instance {
	return domain.Instance{ID: "test-id", Name: request.Spec.Name, Owner: "infinity-iac", Access: "test", Status: domain.StatusProvisioning, Metadata: map[string]string{"infinity-iac-operation": request.IdempotencyKey}}
}

func TestHunterCapacityRotationAndPersistedSuccess(t *testing.T) {
	options := testHunterOptions(t)
	var order []string
	var delays []time.Duration
	base := &lifecycleProvider{profile: options.Profile}
	first := &hunterTestProvider{lifecycleProvider: base, create: func(provider.CreateRequest) (domain.Instance, error) {
		return domain.Instance{}, hunterServiceError{500, "InternalError", "Out of host capacity."}
	}}
	second := &hunterTestProvider{lifecycleProvider: base, create: func(request provider.CreateRequest) (domain.Instance, error) {
		return allocatedHunterInstance(request), nil
	}}
	factory := func(domainName string) (provider.Provider, error) {
		order = append(order, domainName)
		if domainName == "AD-1" {
			return first, nil
		}
		return second, nil
	}
	pause := func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }
	item, err := runHunter(context.Background(), options, factory, pause, io.Discard)
	if err != nil || item.ID != "test-id" || first.calls != 1 || second.calls != 1 || len(delays) != 1 || delays[0] < time.Minute {
		t.Fatalf("capacity rotation failed: %v %v", item, err)
	}
	data, err := os.ReadFile(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	var state hunterState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Pending || state.InstanceID != item.ID || state.Tokens["AD-1"] == state.Tokens["AD-2"] {
		t.Fatal("journal lost the confirmed distinct operation identities")
	}
	info, err := os.Stat(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("journal is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("journal permissions are %o, want 600", info.Mode().Perm())
	}
	base.items = []domain.Instance{item}
	_, err = runHunter(context.Background(), options, factory, pause, io.Discard)
	if err != nil || first.calls != 1 || second.calls != 1 {
		t.Fatalf("confirmed success launched again: %v", err)
	}
}

func TestHunterStopsOnQuotaAndAmbiguousLaunches(t *testing.T) {
	for _, failure := range []error{hunterServiceError{400, "LimitExceeded", "quota exceeded"}, hunterServiceError{401, "NotAuthenticated", "auth failed"}, errors.New("transport timeout")} {
		options := testHunterOptions(t)
		backend := &hunterTestProvider{lifecycleProvider: &lifecycleProvider{profile: options.Profile}, create: func(provider.CreateRequest) (domain.Instance, error) { return domain.Instance{}, failure }}
		pauseCalls := 0
		factory := func(string) (provider.Provider, error) { return backend, nil }
		pause := func(context.Context, time.Duration) error { pauseCalls++; return nil }
		_, err := runHunter(context.Background(), options, factory, pause, io.Discard)
		if err == nil || backend.calls != 1 || pauseCalls != 0 {
			t.Fatalf("terminal or ambiguous launch retried: %v", err)
		}
		_, err = runHunter(context.Background(), options, factory, pause, io.Discard)
		if err == nil || backend.calls != 1 {
			t.Fatal("pending operation launched again without exact reconciliation")
		}
	}
}

func TestHunterRejectsForeignNameAndJournalMismatch(t *testing.T) {
	options := testHunterOptions(t)
	backend := &hunterTestProvider{lifecycleProvider: &lifecycleProvider{profile: options.Profile, items: []domain.Instance{{ID: "personal", Name: options.Spec.Name, Owner: "personal", Access: "test"}}}, create: func(request provider.CreateRequest) (domain.Instance, error) {
		return allocatedHunterInstance(request), nil
	}}
	factory := func(string) (provider.Provider, error) { return backend, nil }
	pause := func(context.Context, time.Duration) error { return nil }
	if _, err := runHunter(context.Background(), options, factory, pause, io.Discard); err == nil || backend.calls != 0 {
		t.Fatal("personal same-name instance was adopted or bypassed")
	}
	options.Spec.RAMGB++
	if _, err := runHunter(context.Background(), options, factory, pause, io.Discard); err == nil || backend.calls != 0 {
		t.Fatal("changed launch settings reused a journal")
	}
}

func TestHunterAttemptLimitAndCancellation(t *testing.T) {
	options := testHunterOptions(t)
	backend := &hunterTestProvider{lifecycleProvider: &lifecycleProvider{profile: options.Profile}, create: func(provider.CreateRequest) (domain.Instance, error) {
		return domain.Instance{}, hunterServiceError{500, "InternalError", "Out of host capacity."}
	}}
	factory := func(string) (provider.Provider, error) { return backend, nil }
	if _, err := runHunter(context.Background(), options, factory, func(context.Context, time.Duration) error { return nil }, io.Discard); err == nil || backend.calls != 3 {
		t.Fatal("attempt limit was not enforced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := hunterPause(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal("backoff did not cancel")
	}
}

func TestHunterDryRunMakesNoCloudCalls(t *testing.T) {
	a, backend, out, stderr := lifecycleApp()
	a.cfg.Profiles[0].AvailabilityDomain = "AD-1"
	a.configPath = filepath.Join(t.TempDir(), "config.json")
	if code := a.ocHunter([]string{"--dry-run", "--output", "json"}); code != 0 || len(backend.created) != 0 {
		t.Fatalf("dry-run failed: %s %s", out, stderr)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(a.configPath), "operations")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run wrote an operation journal")
	}
}
