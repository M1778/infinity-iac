package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultRepository = "M1778/infinity-iac"

func (a *App) update(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(a.err)
	version := fs.String("version", "latest", "release tag or version")
	yes := fs.Bool("yes", false, "confirm replacing the current executable")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return a.fail("update accepts flags only")
	}
	if !*yes && !isTerminal(a.in) {
		return a.fail("update requires --yes in noninteractive mode")
	}
	if !*yes && !a.confirm("update Infinity-IaC to "+*version) {
		return 1
	}
	path, err := os.Executable()
	if err != nil {
		return a.fail("find current executable: " + err.Error())
	}
	if err := updateExecutable(path, *version); err != nil {
		return a.fail(err.Error())
	}
	fmt.Fprintf(a.out, "updated Infinity-IaC at %s\n", path)
	return 0
}

func releaseBase(repository, version string) (string, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(repository, "\\: ") {
		return "", errors.New("repository must be OWNER/REPOSITORY")
	}
	if version == "" || version == "latest" {
		return "https://github.com/" + repository + "/releases/latest/download", nil
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return "https://github.com/" + repository + "/releases/download/" + version, nil
}

func updateExecutable(path, version string) error {
	repository := os.Getenv("INFINITY_IAC_REPOSITORY")
	if repository == "" {
		repository = defaultRepository
	}
	base, err := releaseBase(repository, version)
	if err != nil {
		return err
	}
	asset := "infinity-iac_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	binary, err := downloadRelease(client, base+"/"+asset)
	if err != nil {
		return fmt.Errorf("download release binary: %w", err)
	}
	checksums, err := downloadRelease(client, base+"/SHA256SUMS")
	if err != nil {
		return fmt.Errorf("download release checksums: %w", err)
	}
	expected, err := checksumFor(string(checksums), asset)
	if err != nil {
		return err
	}
	actual := sha256.Sum256(binary)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), expected) {
		return fmt.Errorf("checksum verification failed for %s", asset)
	}
	mode := os.FileMode(0o755)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode() | 0o100
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".infinity-iac-update-*")
	if err != nil {
		return fmt.Errorf("create replacement: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("prepare replacement: %w", err)
	}
	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return fmt.Errorf("write replacement: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync replacement: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close replacement: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace executable (on Windows, close infinity-iac and run update again): %w", err)
	}
	return nil
}

func downloadRelease(client *http.Client, rawURL string) ([]byte, error) {
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

func checksumFor(contents, asset string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(contents))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[1] != asset {
			continue
		}
		value := fields[0]
		if len(value) != sha256.Size*2 {
			return "", fmt.Errorf("invalid checksum for %s", asset)
		}
		if _, err := hex.DecodeString(value); err != nil {
			return "", fmt.Errorf("invalid checksum for %s", asset)
		}
		return strings.ToLower(value), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksum entry for %s is missing", asset)
}
