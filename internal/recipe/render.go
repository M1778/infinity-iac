package recipe

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var inputToken = regexp.MustCompile(`\{\{\s*([a-z][a-z0-9_]*)\s*\}\}`)

func RenderCloudInit(item Recipe, port int) (string, error) {
	return RenderCloudInitWithInputs(item, RenderOptions{Port: port})
}

func RenderCloudInitWithInputs(item Recipe, options RenderOptions) (string, error) {
	if item.Execution == "local" {
		return "", fmt.Errorf("recipe %q executes locally, not through cloud-init", item.Name)
	}
	return renderScript(item, options, false)
}

func RenderLocalScript(item Recipe, options RenderOptions) (string, error) {
	if item.Execution != "local" {
		return "", fmt.Errorf("recipe %q requires an instance", item.Name)
	}
	return renderScript(item, options, true)
}

func renderScript(item Recipe, options RenderOptions, local bool) (string, error) {
	if err := item.Validate(); err != nil {
		return "", err
	}
	if item.Status == "template" {
		return "", fmt.Errorf("recipe %q is an incomplete template: %s", item.Name, strings.Join(item.Requirements, "; "))
	}
	if options.Port < 0 || options.Port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535, or 0 for a script without a service")
	}
	supplied := make(map[string]any, len(options.Inputs)+1)
	for key, value := range options.Inputs {
		supplied[key] = value
	}
	if options.Port > 0 {
		if _, ok := item.Inputs["port"]; ok {
			supplied["port"] = options.Port
		}
	}
	inputs, err := ResolveInputs(item, supplied)
	if err != nil {
		return "", err
	}
	port := options.Port
	if port == 0 {
		if value := inputs["port"]; value != "" {
			port, err = strconv.Atoi(value)
			if err != nil {
				return "", fmt.Errorf("port input must be an integer")
			}
		} else if len(item.Resources.Ingress) > 0 {
			port = item.Resources.Ingress[0].Port
		}
	}
	if port < 0 || port > 65535 || port == 0 && item.Kind == "application" {
		return "", fmt.Errorf("application recipe %q requires a service port between 1 and 65535", item.Name)
	}
	replacePort := func(value string) (string, error) {
		value = strings.ReplaceAll(value, "{{port}}", strconv.Itoa(port))
		if match := inputToken.FindStringSubmatch(value); len(match) > 0 {
			return "", fmt.Errorf("token %s must use a quoted INFINITY_INPUT_ environment variable", match[0])
		}
		return value, nil
	}
	var b strings.Builder
	b.WriteString("#!/bin/bash\nset -euo pipefail\numask 077\n")
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString("export " + inputEnvironment(key, item.Inputs[key]) + "=" + shellQuote(inputs[key]) + "\n")
	}
	if !local {
		b.WriteString("export DEBIAN_FRONTEND=noninteractive\n")
		b.WriteString("exec 9>/var/lock/infinity-iac-bootstrap.lock\nflock -w 300 9\n")
		b.WriteString("install -d -m 0755 /var/lib/infinity-iac/ready\n")
		b.WriteString("rm -f -- " + shellQuote("/var/lib/infinity-iac/ready/"+item.Name) + "\n")
		packages := make([]string, 0)
		seen := make(map[string]bool)
		for _, step := range item.Steps {
			if step.Type == "package" && !seen[step.Package] {
				packages = append(packages, shellQuote(step.Package))
				seen[step.Package] = true
			}
		}
		for _, check := range item.Health {
			if check.Type == "http" && !seen["curl"] {
				packages = append(packages, shellQuote("curl"))
				seen["curl"] = true
			}
		}
		if len(packages) > 0 {
			b.WriteString("timeout --signal=TERM --kill-after=10s 900s apt-get -o DPkg::Lock::Timeout=180 update\n")
			b.WriteString("timeout --signal=TERM --kill-after=10s 900s apt-get -o DPkg::Lock::Timeout=180 install -y -- " + strings.Join(packages, " ") + "\n")
		}
		if seen["docker.io"] {
			b.WriteString("systemctl enable --now docker\n")
		}
	}
	for _, step := range item.Steps {
		switch step.Type {
		case "package":
		case "command":
			command, err := replacePort(step.Command)
			if err != nil {
				return "", fmt.Errorf("step %q: %w", step.ID, err)
			}
			if step.TimeoutSeconds > 0 {
				b.WriteString("timeout --signal=TERM --kill-after=10s " + strconv.Itoa(step.TimeoutSeconds) + "s bash -euo pipefail -c " + shellQuote(command) + "\n")
			} else {
				b.WriteString(command + "\n")
			}
		case "upload", "file":
			data := []byte(step.Content)
			if step.Type == "upload" {
				data, err = os.ReadFile(step.Source)
				if err != nil {
					return "", fmt.Errorf("read upload source %q for step %q: %w", step.Source, step.ID, err)
				}
			}
			mode := step.Mode
			if mode == "" {
				mode = "0600"
			}
			b.WriteString("install -d -- " + shellQuote(filepath.Dir(step.Destination)) + "\n")
			b.WriteString("test ! -L " + shellQuote(step.Destination) + "\n")
			b.WriteString("printf %s " + shellQuote(base64.StdEncoding.EncodeToString(data)) + " | base64 -d > " + shellQuote(step.Destination) + "\n")
			b.WriteString("chmod " + mode + " -- " + shellQuote(step.Destination) + "\n")
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
			checkCommand, err := replacePort(check.Command)
			if err != nil {
				return "", fmt.Errorf("health check %d: %w", i+1, err)
			}
			command = "timeout --signal=TERM --kill-after=2s 15s bash -euo pipefail -c " + shellQuote(checkCommand)
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
	if !local {
		b.WriteString("date -u +%FT%TZ > " + shellQuote("/var/lib/infinity-iac/ready/"+item.Name) + "\n")
	}
	return b.String(), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
