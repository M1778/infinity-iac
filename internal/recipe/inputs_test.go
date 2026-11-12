package recipe

import (
	"encoding/json"
	"math"
	"os/exec"
	"strings"
	"testing"
)

func inputRecipe() Recipe {
	return Recipe{APIVersion: APIVersion, Name: "inputs", Version: "1.0.0", Kind: "script", Execution: "local", Inputs: map[string]Input{
		"message": {Type: "string", Required: true},
		"amount":  {Type: "integer", Default: 2, Min: 1, Max: 10},
		"enabled": {Type: "boolean", Default: false},
	}, Steps: []Step{{ID: "show", Type: "command", Command: `printf '%s' "$INFINITY_INPUT_MESSAGE"`}}}
}

func TestResolveInputsTypesAndRanges(t *testing.T) {
	item := inputRecipe()
	values, err := ResolveInputs(item, map[string]any{"message": "hello", "amount": json.Number("3")})
	if err != nil || values["amount"] != "3" || values["enabled"] != "false" {
		t.Fatalf("resolve inputs = %v, %v", values, err)
	}
	for _, supplied := range []map[string]any{
		{}, {"message": ""}, {"message": 12}, {"message": "ok", "unknown": true},
		{"message": "ok", "amount": 0}, {"message": "ok", "amount": 11},
		{"message": "ok", "amount": 2.5}, {"message": "ok", "amount": math.NaN()},
		{"message": "ok", "amount": json.Number("9223372036854775808")},
		{"message": "ok", "enabled": "true"}, {"message": "\x00"},
	} {
		if _, err := ResolveInputs(item, supplied); err == nil {
			t.Fatalf("invalid supplied inputs were accepted: %v", supplied)
		}
	}
}

func TestInputsCannotChangeTheShellProgram(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	message := "'; printf injected; # $(printf injected)\nquoted \"text\""
	data, err := RenderLocalScript(inputRecipe(), RenderOptions{Inputs: map[string]any{"message": message}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, "-s")
	cmd.Stdin = strings.NewReader(data)
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != message {
		t.Fatalf("literal input changed: output=%q error=%v", out, err)
	}
	if strings.Contains(data, "/var/lib/infinity-iac") || strings.Contains(data, "apt-get") {
		t.Fatal("local recipe must not perform guest bootstrap operations")
	}
}

func TestInputEnvironmentAndSensitiveDefaultsAreValidated(t *testing.T) {
	for _, environment := range []string{"PATH", "BASH_ENV", "INFINITY_INPUT_BAD-NAME", "INFINITY_INPUT_"} {
		item := inputRecipe()
		item.Inputs["message"] = Input{Type: "string", Environment: environment}
		if err := item.Validate(); err == nil {
			t.Fatalf("unsafe environment %q was accepted", environment)
		}
	}
	item := inputRecipe()
	item.Inputs["password"] = Input{Type: "string", Sensitive: true, Default: "not-for-source"}
	if err := item.Validate(); err == nil {
		t.Fatal("sensitive default was accepted")
	}
	item = inputRecipe()
	item.Inputs["alias"] = Input{Type: "string", Environment: "INFINITY_INPUT_MESSAGE"}
	if err := item.Validate(); err == nil {
		t.Fatal("colliding exported input names were accepted")
	}
}

func TestRenderTargetAndLiteralFileContent(t *testing.T) {
	item := inputRecipe()
	if _, err := RenderCloudInit(item, 0); err == nil {
		t.Fatal("local script rendered as guest cloud-init")
	}
	item.Execution = "instance"
	if _, err := RenderLocalScript(item, RenderOptions{}); err == nil {
		t.Fatal("instance script rendered locally")
	}
	item.Inputs = nil
	item.Steps = []Step{{ID: "config", Type: "file", Content: "value=$(touch never-executed)\n", Destination: "/etc/infinity-example.conf", Mode: "0644"}}
	data, err := RenderCloudInit(item, 0)
	if err != nil || !strings.Contains(data, "base64 -d") || !strings.Contains(data, "chmod 0644") || strings.Contains(data, "$(touch") {
		t.Fatalf("file content must be literal base64 data: %s, %v", data, err)
	}
	item.Steps[0].Destination = "relative.conf"
	if err := item.Validate(); err == nil {
		t.Fatal("relative guest destination was accepted")
	}
}

func TestRawTextTokensAreRejected(t *testing.T) {
	item := inputRecipe()
	item.Steps[0].Command = "printf '%s' {{message}}"
	if _, err := RenderLocalScript(item, RenderOptions{Inputs: map[string]any{"message": "hello"}}); err == nil {
		t.Fatal("unsafe raw text interpolation was accepted")
	}
}

func TestCheckSupportRejectsUnsupportedTargets(t *testing.T) {
	item := inputRecipe()
	item.Supports = Supports{Providers: []string{"oracle"}, OS: []string{"ubuntu-24.04"}, Architectures: []string{"arm64"}}
	if err := item.CheckSupport("oci", "ubuntu-24.04", "arm64"); err != nil {
		t.Fatal(err)
	}
	for _, target := range [][3]string{{"aws", "ubuntu-24.04", "arm64"}, {"oracle", "oracle-linux-9", "arm64"}, {"oracle", "ubuntu-24.04", "amd64"}} {
		if err := item.CheckSupport(target[0], target[1], target[2]); err == nil {
			t.Fatalf("unsupported target accepted: %v", target)
		}
	}
}
