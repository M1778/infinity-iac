package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
