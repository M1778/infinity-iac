package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testManifest(t *testing.T, root, directory, data string) {
	t.Helper()
	path := filepath.Join(root, directory)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "recipe.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogPublishesOnlySafeMetadata(t *testing.T) {
	root := t.TempDir()
	testManifest(t, root, "zeta", `api_version: recipe/v1
name: zeta
version: 1.2.0
kind: script
execution: local
status: experimental
description: Local utility
inputs:
  token:
    type: string
    required: true
    default: private-input-secret
steps:
  - id: run
    type: command
    command: private-step-secret
health:
  - type: command
    command: private-health-secret
outputs:
  token:
    type: secret
    value: private-output-secret
`)
	testManifest(t, root, "alpha", `api_version: recipe/v1
name: alpha
version: 1.0.0
kind: application
status: template
requirements: ["Needs an external database"]
supports:
  providers: [oracle]
  architectures: [amd64]
  os: [ubuntu-24.04]
resources:
  cpu: 2
  ram_gb: 4
  storage_gb: 50
`)
	items, err := loadCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items.Recipes) != 2 || items.Recipes[0].Name != "alpha" || items.Recipes[1].Name != "zeta" {
		t.Fatalf("catalog is not complete and sorted: %+v", items)
	}
	if items.Recipes[0].Execution != "instance" || items.Recipes[1].Execution != "local" {
		t.Fatalf("unexpected execution metadata: %+v", items)
	}
	if len(items.Recipes[1].Inputs) != 1 || !items.Recipes[1].Inputs[0].Required {
		t.Fatalf("required input metadata lost: %+v", items.Recipes[1])
	}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-input-secret", "private-step-secret", "private-health-secret", "private-output-secret"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("public metadata includes %s", secret)
		}
	}
	if !strings.Contains(string(data), "Needs an external database") || !strings.Contains(string(data), "recipes/alpha/recipe.yaml") {
		t.Fatal("catalog omitted requirements or source")
	}
}

func TestCatalogRejectsInvalidOrDuplicateMetadata(t *testing.T) {
	for _, name := range []string{"../unsafe", "two words", ""} {
		t.Run("name-"+name, func(t *testing.T) {
			root := t.TempDir()
			testManifest(t, root, "entry", "api_version: recipe/v1\nname: "+name+"\nversion: 1.0.0\nkind: script\n")
			if _, err := loadCatalog(root); err == nil {
				t.Fatal("invalid name accepted")
			}
		})
	}
	root := t.TempDir()
	for _, directory := range []string{"first", "second"} {
		testManifest(t, root, directory, "api_version: recipe/v1\nname: duplicate\nversion: 1.0.0\nkind: script\n")
	}
	if _, err := loadCatalog(root); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate metadata accepted: %v", err)
	}
}

func TestRenderedCatalogEscapesUntrustedText(t *testing.T) {
	data, err := renderEntries(catalog{Recipes: []catalogRecipe{{Name: "sample", Kind: "application", Status: "template", Version: "1.0.0", Execution: "instance", Description: `<script>alert("unsafe")</script>`, Requirements: []string{`<img src=x onerror=alert(1)>`}, Source: "https://github.com/M1778/infinity-iac/blob/master/recipes/sample/recipe.yaml"}}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("<script>")) || bytes.Contains(data, []byte("<img src=x")) {
		t.Fatal("catalog text became executable markup")
	}
	if !bytes.Contains(data, []byte("&lt;script&gt;")) || !bytes.Contains(data, []byte("id=\"recipe-sample\"")) {
		t.Fatal("escaped text or stable catalog anchor missing")
	}
}

func TestGeneratedSectionsAreDeterministicAndChecked(t *testing.T) {
	root := t.TempDir()
	recipes := filepath.Join(root, "recipes")
	testManifest(t, recipes, "sample", "api_version: recipe/v1\nname: sample\nversion: 1.0.0\nkind: script\nstatus: experimental\n")
	page := filepath.Join(root, "index.html")
	output := filepath.Join(root, "catalog.json")
	content := "<!doctype html><html><!-- catalog-summary:start --><!-- catalog-summary:end --><!-- catalog-entries:start --><!-- catalog-entries:end --></html>"
	if err := os.WriteFile(page, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--recipes", recipes, "--out", output, "--page", page}
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(page)
	if !bytes.Equal(first, second) {
		t.Fatal("regeneration changed the page")
	}
	if err := run(append(args, "--check"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(append(args, "--check"), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale generated metadata not rejected: %v", err)
	}
}

func TestGeneratedSectionRequiresExactMarkers(t *testing.T) {
	for _, page := range []string{"no markers", "<!-- test:start --><!-- test:start --><!-- test:end -->", "<!-- test:end --><!-- test:start -->"} {
		if _, err := replaceSection([]byte(page), "test", []byte("entry")); err == nil {
			t.Fatalf("invalid markers accepted: %s", page)
		}
	}
}
