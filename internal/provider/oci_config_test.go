package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectOCIConfigurationPreservesExplicitPathAndProfile(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "config")
	content := "[DEFAULT]\ntenancy=default-tenancy\nregion=us-phoenix-1\n[TEST]\ntenancy=test-tenancy\nregion=us-ashburn-1\n"
	if err := os.WriteFile(explicit, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCI_CONFIG_FILE", filepath.Join(t.TempDir(), "missing"))
	metadata, err := DetectOCIConfiguration(explicit, "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Path != explicit || metadata.Region != "us-ashburn-1" || metadata.TenancyID != "test-tenancy" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	if _, err := DetectOCIConfiguration(explicit, "MISSING"); err == nil {
		t.Fatal("missing named profile must not fall back to DEFAULT")
	}
}

func TestDetectOCIConfigDoesNotFallBackFromExplicitOrEnvironment(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := DetectOCIConfiguration(missing, "DEFAULT"); err == nil {
		t.Fatal("missing explicit config must fail")
	}
	t.Setenv("OCI_CONFIG_FILE", missing)
	if _, err := DetectOCIConfiguration("", "DEFAULT"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("invalid environment config must fail: %v", err)
	}
}

func TestDetectOCIConfigurationUsesEnvironmentAndRejectsDirectories(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config")
	if err := os.WriteFile(path, []byte("[DEFAULT]\nregion=us-phoenix-1\ntenancy=example-tenancy\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCI_CONFIG_FILE", path)
	metadata, err := DetectOCIConfiguration("", "DEFAULT")
	if err != nil || metadata.Path != path || metadata.Region != "us-phoenix-1" {
		t.Fatalf("environment config: %+v %v", metadata, err)
	}
	if _, err := DetectOCIConfiguration(directory, "DEFAULT"); err == nil {
		t.Fatal("directory accepted as config")
	}
}

func TestReadOCIConfigExpandsHomeAndRejectsHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, "config")
	if err := os.WriteFile(path, []byte("[DEFAULT]\nregion=us-phoenix-1\ntenancy=example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := readableOCIConfigPath("~/config"); err != nil || got != path {
		t.Fatalf("expanded path: %s %v", got, err)
	}
	if _, err := readableOCIConfigPath("~"); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("home directory must not be read as config: %v", err)
	}
}
