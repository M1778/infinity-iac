package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func call(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	code := RunWithIO(args, bytes.NewBuffer(nil), &out, &err)
	return out.String(), err.String(), code
}

func TestSetupCreateListAndLifecycle(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	if _, stderr, code := call(t, "setup", "oracle", "--name", "default", "--profile", "DEFAULT", "--region", "local"); code != 0 {
		t.Fatalf("setup failed: %d %s", code, stderr)
	}
	out, stderr, code := call(t, "create", "server", "--name", "demo", "--provider", "oracle", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("create failed: %d %s", code, stderr)
	}
	var created []map[string]any
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("invalid create JSON: %v", err)
	}
	if len(created) != 1 || created[0]["name"] != "demo" {
		t.Fatalf("unexpected create result: %s", out)
	}
	if _, stderr, code := call(t, "instance", "stop", "demo"); code != 0 {
		t.Fatalf("stop failed: %d %s", code, stderr)
	}
	if _, stderr, code := call(t, "instance", "restart", "demo"); code != 0 {
		t.Fatalf("restart failed: %d %s", code, stderr)
	}
	if _, stderr, code := call(t, "instance", "delete", "demo", "--yes"); code != 0 {
		t.Fatalf("delete failed: %d %s", code, stderr)
	}
	out, stderr, code = call(t, "list", "instances", "--output", "json")
	if code != 0 {
		t.Fatalf("list failed: %d %s", code, stderr)
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("invalid list JSON: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("deleted instance still listed: %s", out)
	}
}

func TestCreateRequiresYesWhenInputIsNotTerminal(t *testing.T) {
	t.Setenv("INFINITY_IAC_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	if _, _, code := call(t, "setup", "oracle", "--name", "default"); code != 0 {
		t.Fatal("setup failed")
	}
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
	if _, _, code := call(t, "setup", "oracle", "--name", "default", "--region", "local"); code != 0 {
		t.Fatal("setup failed")
	}
	manifest := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(manifest, []byte("api_version: v1\nservers:\n  - name: yaml-demo\n    cpu: 2\n    ram_gb: 2\n    storage_gb: 40\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := call(t, "plan", "--file", manifest, "--output", "json")
	if code != 0 {
		t.Fatalf("plan failed: %d %s", code, stderr)
	}
	var plan map[string]any
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	hash, ok := plan["hash"].(string)
	if !ok || hash == "" {
		t.Fatalf("missing plan hash: %s", out)
	}
	if _, stderr, code := call(t, "apply", "--file", manifest, "--hash", hash, "--yes"); code != 0 {
		t.Fatalf("apply failed: %d %s", code, stderr)
	}
}
