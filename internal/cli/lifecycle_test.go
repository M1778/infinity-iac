package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M1778/infinity-iac/internal/config"
	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/M1778/infinity-iac/internal/provider"
)

type lifecycleProvider struct {
	profile domain.AccessProfile
	items   []domain.Instance
	created []provider.CreateRequest
	actions []string
}

func (p *lifecycleProvider) Name() string                  { return "oracle" }
func (p *lifecycleProvider) Profile() domain.AccessProfile { return p.profile }
func (p *lifecycleProvider) Architecture() string          { return "arm64" }
func (p *lifecycleProvider) OperatingSystem(context.Context) (string, error) {
	return "ubuntu-24.04", nil
}
func (p *lifecycleProvider) ListInstances(context.Context, provider.Filter) ([]domain.Instance, error) {
	return p.items, nil
}
func (p *lifecycleProvider) CreateInstance(_ context.Context, request provider.CreateRequest) (domain.Instance, error) {
	p.created = append(p.created, request)
	return domain.Instance{ID: "managed-id", Name: request.Spec.Name, Status: domain.StatusProvisioning, Owner: "infinity-iac", Access: p.profile.Name}, nil
}
func (p *lifecycleProvider) GetInstance(_ context.Context, id string) (domain.Instance, error) {
	p.actions = append(p.actions, "get:"+id)
	return domain.Instance{ID: id, Name: "managed", Status: domain.StatusRunning}, nil
}
func (p *lifecycleProvider) StartInstance(_ context.Context, id string) (domain.Instance, error) {
	p.actions = append(p.actions, "start:"+id)
	return domain.Instance{ID: id, Name: "managed", Status: domain.StatusProvisioning}, nil
}
func (p *lifecycleProvider) StopInstance(_ context.Context, id string) (domain.Instance, error) {
	p.actions = append(p.actions, "stop:"+id)
	return domain.Instance{ID: id, Name: "managed", Status: domain.StatusProvisioning}, nil
}
func (p *lifecycleProvider) RestartInstance(_ context.Context, id string) (domain.Instance, error) {
	p.actions = append(p.actions, "restart:"+id)
	return domain.Instance{ID: id, Name: "managed", Status: domain.StatusProvisioning}, nil
}
func (p *lifecycleProvider) DeleteInstance(_ context.Context, id string) error {
	p.actions = append(p.actions, "delete:"+id)
	return nil
}
func (p *lifecycleProvider) DeleteInstanceAndWait(_ context.Context, id string) error {
	p.actions = append(p.actions, "delete-wait:"+id)
	return nil
}
func (p *lifecycleProvider) WaitInstance(_ context.Context, id, status string) (domain.Instance, error) {
	p.actions = append(p.actions, "wait:"+id+":"+status)
	return domain.Instance{ID: id, Name: "managed", Status: status}, nil
}

func lifecycleApp() (*App, *lifecycleProvider, *bytes.Buffer, *bytes.Buffer) {
	profile := domain.AccessProfile{Name: "test", Provider: "oracle", Region: "us-phoenix-1", Shape: "VM.Standard.A1.Flex"}
	backend := &lifecycleProvider{profile: profile, items: []domain.Instance{{ID: "managed-id", Name: "managed", Access: "test", Owner: "infinity-iac", Status: domain.StatusRunning}}}
	out, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	return &App{in: strings.NewReader(""), out: out, err: stderr, cfg: config.File{DefaultAccess: "test", Profiles: []domain.AccessProfile{profile}}, p: backend}, backend, out, stderr
}

func TestInstanceFlagsOnEitherSideOfIdentifier(t *testing.T) {
	for _, args := range [][]string{
		{"get", "managed", "--output", "json", "--access", "test"},
		{"get", "--output", "json", "--access", "test", "managed"},
		{"start", "managed", "--yes", "--wait", "--output", "json"},
		{"stop", "--access=test", "managed", "--yes", "--wait"},
		{"restart", "managed", "--timeout", "1m", "--yes", "--wait"},
		{"delete", "managed", "--output", "json", "--yes", "--wait"},
	} {
		a, backend, out, stderr := lifecycleApp()
		if code := a.instance(args); code != 0 || len(backend.actions) == 0 {
			t.Fatalf("%v: code=%d stderr=%s stdout=%s", args, code, stderr, out)
		}
	}
}

func TestInstanceRejectsUnmanagedAndUnconfirmedActions(t *testing.T) {
	for _, args := range [][]string{
		{"delete", "personal", "--yes"}, {"stop", "managed"}, {"start", "managed"},
		{"get", "managed", "--output", "invalid"}, {"get", "managed", "--timeout", "0s"},
		{"get", "managed", "--timeout"}, {"delete", "managed", "other", "--yes"},
	} {
		a, backend, _, stderr := lifecycleApp()
		if code := a.instance(args); code == 0 || len(backend.actions) != 0 {
			t.Fatalf("unsafe invocation %v: code=%d actions=%v stderr=%s", args, code, backend.actions, stderr)
		}
	}
}

func TestInstanceDeletionReportsAcceptanceSeparately(t *testing.T) {
	a, _, out, stderr := lifecycleApp()
	if code := a.instance([]string{"delete", "managed", "--yes", "--output", "json"}); code != 0 {
		t.Fatalf("%d %s", code, stderr)
	}
	var result struct {
		Status    string `json:"status"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Status != "terminating" || result.Confirmed {
		t.Fatalf("deletion acceptance mislabeled: %s %v", out, err)
	}
}

func TestCreateUsesInjectedProviderAndUniqueOperationIdentity(t *testing.T) {
	a, backend, _, stderr := lifecycleApp()
	for i := 0; i < 2; i++ {
		if code := a.createServer([]string{"--name", "example", "--access", "test", "--yes"}, "server", "#!/bin/bash\nprintf ready\n"); code != 0 {
			t.Fatalf("code=%d stderr=%s", code, stderr)
		}
	}
	if len(backend.created) != 2 || backend.created[0].IdempotencyKey == backend.created[1].IdempotencyKey || backend.created[0].UserData == "" {
		t.Fatal("new create operations must retain bootstrap and distinct identities")
	}
}

func TestPlanHashIncludesStartupFileContents(t *testing.T) {
	a, backend, out, stderr := lifecycleApp()
	backend.items = nil
	dir := t.TempDir()
	startup := filepath.Join(dir, "startup.sh")
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(startup, []byte("#!/bin/bash\nprintf one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"api_version":"v1","servers":[{"name":"new","startup_script":"startup.sh"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := a.plan([]string{"--file", manifest, "--output", "json"}, false); code != 0 {
		t.Fatalf("plan code=%d stderr=%s", code, stderr)
	}
	var first domain.Plan
	if err := json.Unmarshal(out.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := os.WriteFile(startup, []byte("#!/bin/bash\nprintf two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := a.plan([]string{"--file", manifest, "--hash", first.Hash, "--yes"}, true); code == 0 || len(backend.created) != 0 {
		t.Fatal("changed startup file applied under stale reviewed hash")
	}
	stderr.Reset()
	if code := a.plan([]string{"--file", manifest, "--output", "json"}, false); code != 0 {
		t.Fatal(stderr.String())
	}
	var second domain.Plan
	if err := json.Unmarshal(out.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := a.plan([]string{"--file", manifest, "--hash", second.Hash, "--yes"}, true); code != 0 {
		t.Fatal(stderr.String())
	}
	if len(backend.created) != 1 || !strings.Contains(backend.created[0].UserData, "two") {
		t.Fatal("approved bootstrap was not sent to provider")
	}
}
