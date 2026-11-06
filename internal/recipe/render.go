package recipe

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RenderCloudInit produces a bounded bootstrap script without contacting a provider.
func RenderCloudInit(item Recipe, port int) (string, error) {
	if err := item.Validate(); err != nil {
		return "", err
	}
	if item.Status == "template" {
		return "", fmt.Errorf("recipe %q is an incomplete template: %s", item.Name, strings.Join(item.Requirements, "; "))
	}
	if port < 0 || port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535, or 0 for a script without a service")
	}
	if port == 0 && item.Kind == "application" {
		return "", fmt.Errorf("application recipe %q requires a service port", item.Name)
	}
	replacePort := func(value string) string {
		return strings.ReplaceAll(value, "{{port}}", strconv.Itoa(port))
	}
	var b strings.Builder
	b.WriteString("#!/bin/bash\nset -euo pipefail\nexport DEBIAN_FRONTEND=noninteractive\n")
	b.WriteString("exec 9>/var/lock/infinity-iac-bootstrap.lock\nflock -w 300 9\n")
	b.WriteString("install -d -m 0755 /var/lib/infinity-iac/ready\n")
	b.WriteString("rm -f -- " + shellQuote("/var/lib/infinity-iac/ready/"+item.Name) + "\n")
	packages := make([]string, 0)
	seenPackages := make(map[string]bool)
	for _, step := range item.Steps {
		if step.Type == "package" && !seenPackages[step.Package] {
			packages = append(packages, shellQuote(step.Package))
			seenPackages[step.Package] = true
		}
	}
	for _, check := range item.Health {
		if check.Type == "http" && !seenPackages["curl"] {
			packages = append(packages, shellQuote("curl"))
			seenPackages["curl"] = true
		}
	}
	if len(packages) > 0 {
		b.WriteString("apt-get -o DPkg::Lock::Timeout=180 update\napt-get -o DPkg::Lock::Timeout=180 install -y -- " + strings.Join(packages, " ") + "\n")
	}
	if seenPackages["docker.io"] {
		b.WriteString("systemctl enable --now docker\n")
	}
	for _, step := range item.Steps {
		switch step.Type {
		case "package":
		case "command":
			command := replacePort(step.Command)
			if step.TimeoutSeconds > 0 {
				b.WriteString("timeout --signal=TERM --kill-after=10s " + strconv.Itoa(step.TimeoutSeconds) + "s bash -euo pipefail -c " + shellQuote(command) + "\n")
			} else {
				b.WriteString(command + "\n")
			}
		case "upload":
			data, err := os.ReadFile(step.Source)
			if err != nil {
				return "", fmt.Errorf("read upload source %q for step %q: %w", step.Source, step.ID, err)
			}
			b.WriteString("install -d -- " + shellQuote(filepath.Dir(step.Destination)) + "\n")
			b.WriteString("printf %s " + shellQuote(base64.StdEncoding.EncodeToString(data)) + " | base64 -d > " + shellQuote(step.Destination) + "\n")
		}
	}
	for i, check := range item.Health {
		timeout := check.TimeoutSeconds
		if timeout < 1 {
			timeout = 180
		}
		b.WriteString("health_deadline=$((SECONDS + " + strconv.Itoa(timeout) + "))\n")
		var command string
		switch check.Type {
		case "command":
			command = "timeout --signal=TERM --kill-after=2s 15s bash -euo pipefail -c " + shellQuote(replacePort(check.Command))
		case "http":
			path := check.Path
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			healthPort := check.Port
			if port > 0 {
				healthPort = port
			}
			url := "http://127.0.0.1:" + strconv.Itoa(healthPort) + path
			command = "curl --fail --silent --show-error --connect-timeout 3 --max-time 10 " + shellQuote(url) + " >/dev/null"
		}
		b.WriteString("until " + command + "; do\n")
		b.WriteString("  if (( SECONDS >= health_deadline )); then\n")
		b.WriteString("    printf '%s\\n' " + shellQuote(fmt.Sprintf("health check %d failed for recipe %s", i+1, item.Name)) + " >&2\n    exit 1\n  fi\n  sleep 3\ndone\n")
	}
	b.WriteString("date -u +%FT%TZ > " + shellQuote("/var/lib/infinity-iac/ready/"+filepath.Base(item.Name)) + "\n")
	return b.String(), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
