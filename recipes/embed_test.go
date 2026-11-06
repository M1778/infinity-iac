package recipes

import (
	"strings"
	"testing"

	"github.com/M1778/infinity-iac/internal/recipe"
)

func TestEmbeddedCatalogIsValid(t *testing.T) {
	items, err := recipe.DiscoverFS(FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 100 {
		t.Fatalf("embedded catalog has %d recipes; want at least 100", len(items))
	}
	if _, err := recipe.NewCatalog(items); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogHasScriptsAndPrivateProtocolChecks(t *testing.T) {
	items, err := recipe.DiscoverFS(FS)
	if err != nil {
		t.Fatal(err)
	}
	scripts, applications := 0, 0
	for _, item := range items {
		if item.Status == "" {
			t.Errorf("%s has no readiness status", item.Name)
		}
		if item.Kind == "script" {
			scripts++
			continue
		}
		applications++
		for _, ingress := range item.Resources.Ingress {
			if ingress.Source != "loopback" {
				t.Errorf("%s defaults to public ingress", item.Name)
			}
		}
		if item.Status == "template" {
			continue
		}
		commands := ""
		for _, step := range item.Steps {
			commands += step.Command + "\n"
		}
		if !strings.Contains(commands, "-p 127.0.0.1:") {
			t.Errorf("%s does not bind on loopback", item.Name)
		}
		if !strings.Contains(commands, "Refusing unowned container") {
			t.Errorf("%s does not protect existing containers", item.Name)
		}
		if item.Name != "nginx" && item.Name != "excalidraw" && !strings.Contains(commands, "docker volume create") {
			t.Errorf("%s has no stable persistent volume", item.Name)
		}
		if len(item.Health) == 0 {
			t.Errorf("%s has no protocol readiness check", item.Name)
		}
		for _, health := range item.Health {
			if strings.Contains(health.Command, ".State.Running") {
				t.Errorf("%s checks only process liveness", item.Name)
			}
		}
	}
	if scripts < 5 || applications < 100 {
		t.Fatalf("catalog missing apps/scripts: %d apps, %d scripts", applications, scripts)
	}
}
