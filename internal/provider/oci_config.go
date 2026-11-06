package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type OCIConfigurationMetadata struct {
	Path      string
	Region    string
	TenancyID string
}

func DetectOCIConfiguration(explicitPath, profile string) (OCIConfigurationMetadata, error) {
	if profile == "" {
		profile = "DEFAULT"
	}
	path, err := detectOCIConfigPath(explicitPath)
	if err != nil {
		return OCIConfigurationMetadata{}, err
	}
	configuration, err := common.ConfigurationProviderFromFileWithProfile(path, profile, "")
	if err != nil {
		return OCIConfigurationMetadata{}, fmt.Errorf("read OCI profile %q: %w", profile, err)
	}
	region, err := configuration.Region()
	if err != nil {
		return OCIConfigurationMetadata{}, fmt.Errorf("read region from OCI profile %q: %w", profile, err)
	}
	tenancy, err := configuration.TenancyOCID()
	if err != nil {
		return OCIConfigurationMetadata{}, fmt.Errorf("read tenancy from OCI profile %q: %w", profile, err)
	}
	return OCIConfigurationMetadata{Path: path, Region: region, TenancyID: tenancy}, nil
}

func detectOCIConfigPath(explicitPath string) (string, error) {
	if explicitPath != "" {
		return readableOCIConfigPath(explicitPath)
	}
	if path := os.Getenv("OCI_CONFIG_FILE"); path != "" {
		return readableOCIConfigPath(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve OCI config directory: %w", err)
	}
	candidates := []string{filepath.Join(home, ".oci", "config")}
	if sudoHome, ok := sudoUserHome(); ok && sudoHome != home {
		candidates = append(candidates, filepath.Join(sudoHome, ".oci", "config"))
	}
	if runtime.GOOS != "windows" && home != "/root" {
		candidates = append(candidates, "/root/.oci/config")
	}
	var failures []string
	for _, path := range candidates {
		resolved, err := readableOCIConfigPath(path)
		if err == nil {
			return resolved, nil
		}
		failures = append(failures, err.Error())
	}
	return "", fmt.Errorf("no readable OCI config found; use --config PATH or run with the user that owns the key (%s)", strings.Join(failures, "; "))
}

func readableOCIConfigPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	file, err := os.Open(absolute)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", absolute, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("OCI config %s is not a regular file", absolute)
	}
	return absolute, nil
}
