package recipe

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryRecipeRendersValidShell(t *testing.T) {
	items, err := Discover(filepath.Join("..", "..", "recipes"))
	if err != nil {
		t.Fatal(err)
	}
	shell, shellErr := exec.LookPath("bash")
	for _, item := range items {
		t.Run(item.Name, func(t *testing.T) {
			port := 0
			if len(item.Resources.Ingress) > 0 {
				port = item.Resources.Ingress[0].Port
			}
			data, err := RenderCloudInit(item, port)
			if item.Status == "template" {
				if err == nil {
					t.Fatal("incomplete templates must not render")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(data, "set -euo pipefail") || !strings.Contains(data, "/var/lib/infinity-iac/ready/") {
				t.Fatal("bootstrap must fail on errors and record completed health checks")
			}
			if shellErr == nil {
				cmd := exec.Command(shell, "-n")
				cmd.Stdin = strings.NewReader(data)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("invalid shell: %v\n%s", err, output)
				}
			}
		})
	}
}

func TestRenderUsesBoundedHealthAndStepTimeout(t *testing.T) {
	item := Recipe{APIVersion: APIVersion, Name: "sample", Version: "1.0.0", Kind: "application",
		Steps:  []Step{{ID: "install", Type: "package", Package: "docker.io"}, {ID: "run", Type: "command", Command: "echo {{port}}", TimeoutSeconds: 20}},
		Health: []HealthCheck{{Type: "http", Port: 80, Path: "/health", TimeoutSeconds: 30}}}
	data, err := RenderCloudInit(item, 1080)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"systemctl enable --now docker", "20s bash", "health_deadline=$((SECONDS + 30))", "127.0.0.1:1080/health", "--connect-timeout 3 --max-time 10"} {
		if !strings.Contains(data, required) {
			t.Errorf("missing %q", required)
		}
	}
	if _, err := RenderCloudInit(item, 65536); err == nil {
		t.Fatal("out-of-range port was accepted")
	}
}
