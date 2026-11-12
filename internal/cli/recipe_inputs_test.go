package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSensitiveInputsRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	dir := t.TempDir()
	manifest := filepath.Join(dir, "recipe.yaml")
	values := filepath.Join(dir, "inputs.json")
	data := "api_version: recipe/v1\nname: configured\nversion: 1.0.0\nkind: script\nexecution: local\ninputs:\n  password:\n    type: string\n    required: true\n    sensitive: true\nsteps:\n  - id: check\n    type: command\n    command: 'test -n \"$INFINITY_INPUT_PASSWORD\"'\n"
	if err := os.WriteFile(manifest, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(values, []byte(`{"password":"literal-test-only-value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := call(t, "recipe", "render", "--file", manifest, "--inputs", values)
	if code == 0 || out != "" || !strings.Contains(stderr, "--include-sensitive") || strings.Contains(stderr, "literal-test-only-value") {
		t.Fatalf("sensitive render guard failed: %d %q %q", code, out, stderr)
	}
	out, stderr, code = call(t, "recipe", "render", "--file", manifest, "--inputs", values, "--include-sensitive")
	if code != 0 || !strings.Contains(out, "INFINITY_INPUT_PASSWORD=") {
		t.Fatalf("explicit sensitive render failed: %d %s", code, stderr)
	}
}

func TestInputFileErrorsDoNotEchoValues(t *testing.T) {
	for _, text := range []string{`{"token":"private-value",}`, `[{"token":"private-value"}]`, `null`, `{} {}`} {
		path := filepath.Join(t.TempDir(), "inputs.json")
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := loadRecipeInputs(path)
		if err == nil || strings.Contains(err.Error(), "private-value") {
			t.Fatalf("input boundary leaked or accepted invalid object: %v", err)
		}
	}
}

func TestRunDryRunDoesNotNeedCloudCredentials(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	out, stderr, code := call(t, "run", "--app", "nginx", "--dry-run", "--output", "json")
	if code != 0 || !strings.Contains(out, `"dry_run": true`) {
		t.Fatalf("dry-run failed: %d %s %s", code, out, stderr)
	}
	_, stderr, code = call(t, "run", "--script", "nginx", "--dry-run")
	if code == 0 || !strings.Contains(stderr, "kind") {
		t.Fatalf("wrong recipe kind accepted: %d %s", code, stderr)
	}
	_, stderr, code = call(t, "run", "--app", "nginx", "--local", "--yes")
	if code == 0 || !strings.Contains(stderr, "requires an instance") {
		t.Fatalf("guest recipe allowed locally: %d %s", code, stderr)
	}
}
