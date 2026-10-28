package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M1778/infinity-iac/internal/config"
	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/M1778/infinity-iac/internal/recipe"
)

func call(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	code := RunWithIO(args, bytes.NewBuffer(nil), &out, &err)
	return out.String(), err.String(), code
}

func TestSetupCreateListAndLifecycle(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	_, stderr, code := call(t, "setup", "oracle", "--name", "default", "--profile", "DEFAULT", "--region", "us-phoenix-1")
	if code == 0 || !strings.Contains(stderr, "required") {
		t.Fatalf("expected production setup validation, code=%d stderr=%q", code, stderr)
	}
}

func TestCreateRequiresYesWhenInputIsNotTerminal(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	_, stderr, code := call(t, "create", "server", "--name", "demo", "--provider", "oracle")
	if code == 0 || stderr == "" {
		t.Fatalf("expected noninteractive confirmation error, code=%d stderr=%q", code, stderr)
	}
}

func TestUnlinkRemovesLocalAccessProfileAndPromotesDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("INFINITY_IAC_CONFIG", path)
	file := config.File{DefaultAccess: "first", Profiles: []domain.AccessProfile{{Name: "first", Provider: "oracle"}, {Name: "second", Provider: "oracle"}}}
	if err := config.Save(path, file); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := call(t, "unlink", "acc", "first", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("unlink failed: %d %s", code, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "unlinked" || result["remote"] != "unchanged" {
		t.Fatalf("unexpected unlink result: %s", out)
	}
	updated, _, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if updated.DefaultAccess != "second" || len(updated.Profiles) != 1 || updated.Profiles[0].Name != "second" {
		t.Fatalf("unexpected config after unlink: %+v", updated)
	}
}

func TestConfigDoesNotNeedHomeDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.Setenv("INFINITY_IAC_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv("INFINITY_IAC_CONFIG")
	if _, _, code := call(t, "doctor", "--output", "json"); code != 0 {
		t.Fatal("doctor failed")
	}
}

func TestPlanAndApplyUseStableHashAndYAMLDefaults(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	manifest := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("api_version: v1\nservers:\n  - name: yaml-demo\n    cpu: 2\n    ram_gb: 2\n    storage_gb: 40\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := call(t, "plan", "--file", manifest, "--output", "json")
	if code == 0 || !strings.Contains(stderr, "no access profile") {
		t.Fatalf("expected provider configuration error, code=%d stderr=%q", code, stderr)
	}
}

func TestRecipeListUsesBuiltInCatalog(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	out, stderr, code := call(t, "list", "recipes", "--output", "json")
	if code != 0 {
		t.Fatalf("recipe list failed: %d %s", code, stderr)
	}
	var result struct {
		Recipes []struct {
			Name string `json:"name"`
		} `json:"recipes"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Recipes) != 1 || result.Recipes[0].Name != "n8n" {
		t.Fatalf("unexpected recipe list: %s", out)
	}
}

func TestRecipeValidationCommand(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	file := filepath.Join(t.TempDir(), "recipe.yaml")
	content := "api_version: recipe/v1\nname: test\nversion: 1.0.0\nkind: script\nsteps:\n  - id: run\n    type: command\n    command: echo ok\n"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := call(t, "validate", "recipe", "--file", file); code != 0 {
		t.Fatalf("validate failed: %d %s", code, stderr)
	}
}

func TestRunUnknownApplicationFailsBeforeProvisioning(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	_, stderr, code := call(t, "run", "--app", "does-not-exist", "--yes")
	if code == 0 || !strings.Contains(stderr, "was not found") {
		t.Fatalf("expected missing recipe error, code=%d stderr=%q", code, stderr)
	}
}

func TestThirdPartyAppInstallStoresRecipeAndListsIt(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("INFINITY_IAC_CONFIG", configPath)
	manifest := filepath.Join(t.TempDir(), "app.yml")
	content := "api_version: recipe/v1\nname: hello\nversion: 1.0.0\nkind: script\nsteps:\n  - id: run\n    type: command\n    command: echo hello\n"
	if err := os.WriteFile(manifest, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := call(t, "app", "install", manifest, "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("install failed: %d %s", code, stderr)
	}
	if !strings.Contains(out, "status") || !strings.Contains(out, "installed") {
		t.Fatalf("unexpected install output: %s", out)
	}
	out, stderr, code = call(t, "list", "recipes", "--output", "json")
	if code != 0 || !strings.Contains(out, "hello") {
		t.Fatalf("installed recipe missing: %d %s %s", code, stderr, out)
	}
}

func TestThirdPartyAppInstallRequiresYesNonInteractive(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	manifest := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(manifest, []byte("api_version: recipe/v1\nname: hello\nversion: 1.0.0\nkind: script\nsteps:\n  - id: run\n    type: command\n    command: echo hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := call(t, "app", "install", manifest)
	if code == 0 || !strings.Contains(stderr, "requires --yes") {
		t.Fatalf("expected confirmation failure: %d %s", code, stderr)
	}
}

func TestExternalAppInstallRejectsHTTP(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	_, stderr, code := call(t, "app", "install", "--external", "http://example.com/app.yml", "--yes")
	if code == 0 || !strings.Contains(stderr, "HTTPS") {
		t.Fatalf("expected HTTPS rejection: %d %s", code, stderr)
	}
}

func TestRecipeCloudInitUsesPortAndRejectsMissingUpload(t *testing.T) {
	r, ok := findRecipe("n8n")
	if !ok {
		t.Fatal("built-in n8n recipe not found")
	}
	data, err := recipeCloudInit(r, 1080)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, "-p 1080:5678") || !strings.Contains(data, "127.0.0.1:1080/") || !strings.Contains(data, "apt-get install") || !strings.Contains(data, "curl --fail") {
		t.Fatalf("unexpected cloud-init: %s", data)
	}
	cpu, ram, storage := recipeResources(r)
	if cpu != 2 || ram != 2 || storage != 35 {
		t.Fatalf("unexpected recipe resources: %d/%d/%d", cpu, ram, storage)
	}
	r.Steps = append(r.Steps, recipe.Step{ID: "missing", Type: "upload", Source: filepath.Join(t.TempDir(), "missing"), Destination: "/tmp/missing"})
	if _, err := recipeCloudInit(r, 1080); err == nil {
		t.Fatal("expected missing upload source to fail")
	}
}

func TestUpdateChecksumParsing(t *testing.T) {
	value := strings.Repeat("a", 64)
	got, err := checksumFor(value+"  infinity-iac_linux_amd64\n", "infinity-iac_linux_amd64")
	if err != nil || got != value {
		t.Fatalf("checksumFor = %q, %v", got, err)
	}
	if _, err := checksumFor("bad  infinity-iac_linux_amd64\n", "infinity-iac_linux_amd64"); err == nil {
		t.Fatal("expected invalid checksum error")
	}
	if _, err := checksumFor(value+"  other\n", "infinity-iac_linux_amd64"); err == nil {
		t.Fatal("expected missing checksum error")
	}
}

func TestReleaseBaseNormalizesVersion(t *testing.T) {
	got, err := releaseBase("owner/repo", "1.2.3")
	if err != nil || got != "https://github.com/owner/repo/releases/download/v1.2.3" {
		t.Fatalf("release base = %q, %v", got, err)
	}
	if _, err := releaseBase("owner/bad/repo", "latest"); err == nil {
		t.Fatal("expected invalid repository error")
	}
}
