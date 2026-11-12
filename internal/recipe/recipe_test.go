package recipe

import (
	"path/filepath"
	"testing"
)

func TestLoadAndPlanN8NRecipe(t *testing.T) {
	r, err := Load(filepath.Join("..", "..", "recipes", "n8n", "recipe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "n8n" || r.Resources.Specs != "ibase2" {
		t.Fatalf("unexpected recipe: %+v", r)
	}
	plan := r.Plan()
	if len(plan.Steps) != 2 || plan.Steps[1].Status != "pending" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestValidateRejectsInvalidRecipe(t *testing.T) {
	r := Recipe{APIVersion: APIVersion, Name: "bad", Version: "1.0.0", Kind: "application", Steps: []Step{{ID: "x", Type: "command"}}}
	if err := r.Validate(); err == nil {
		t.Fatal("expected invalid command to fail")
	}
}

func TestValidateRejectsUnsafeShapeValues(t *testing.T) {
	tests := []Recipe{
		{APIVersion: APIVersion, Name: "bad", Version: "1.0.0", Kind: "script", Supports: Supports{Providers: []string{""}}, Steps: []Step{{ID: "x", Type: "command", Command: "true"}}},
		{APIVersion: APIVersion, Name: "bad", Version: "1.0.0", Kind: "application", Steps: []Step{{ID: "x", Type: "command", Command: "true"}}, Health: []HealthCheck{{Type: "http", Port: 80}}},
	}
	for _, item := range tests {
		if err := item.Validate(); err == nil {
			t.Fatal("expected invalid recipe shape to fail")
		}
	}
}

func TestDiscoverRecipes(t *testing.T) {
	items, err := Discover(filepath.Join("..", "..", "recipes"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 100 {
		t.Fatalf("unexpected recipe count: %d", len(items))
	}
}

func TestCatalogRejectsDuplicateNames(t *testing.T) {
	r := Recipe{APIVersion: APIVersion, Name: "x", Version: "1.0.0", Kind: "script", Steps: []Step{{ID: "run", Type: "command", Command: "true"}}}
	if _, err := NewCatalog([]Recipe{r, r}); err == nil {
		t.Fatal("expected duplicate recipe name to fail")
	}
}

func TestRecipeNamesAreSafeForReadinessFiles(t *testing.T) {
	for _, name := range []string{"../bad", "bad/name", ".", "..", "bad name", "bad\nname"} {
		r := Recipe{APIVersion: APIVersion, Name: name, Version: "1.0.0", Kind: "script", Steps: []Step{{ID: "run", Type: "command", Command: "true"}}}
		if err := r.Validate(); err == nil {
			t.Fatalf("unsafe recipe name %q accepted", name)
		}
	}
}

func TestTemplateRequiresDocumentedRequirements(t *testing.T) {
	r := Recipe{APIVersion: APIVersion, Name: "template", Version: "1.0.0", Kind: "application", Status: "template", Steps: []Step{{ID: "run", Type: "command", Command: "true"}}}
	if err := r.Validate(); err == nil {
		t.Fatal("template without requirements was accepted")
	}
	r.Requirements = []string{"Configure a database"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderCloudInit(r, 80); err == nil {
		t.Fatal("incomplete template was rendered")
	}
}
