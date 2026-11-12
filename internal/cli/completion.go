package cli

import (
	"fmt"
	"io"
	"strings"
)

var topLevelCommands = []string{"setup", "unlink", "list", "create", "run", "network", "instance", "oc-hunter", "install", "app", "update", "plan", "apply", "recipe", "validate", "doctor", "completion", "version", "help"}

// complete is a shell-neutral protocol: one candidate per line. Shell
// adapters do prefix filtering and turn these candidates into native menus.
func complete(args []string, out io.Writer) int {
	words := args
	if len(words) > 0 && words[0] == "--" {
		words = words[1:]
	}
	if len(words) > 0 && words[0] == "infinity-iac" {
		words = words[1:]
	}
	candidates := completionCandidates(words)
	for _, candidate := range candidates {
		fmt.Fprintln(out, candidate)
	}
	return 0
}

func completionCandidates(words []string) []string {
	if len(words) == 0 {
		return topLevelCommands
	}
	command := words[0]
	if len(words) > 1 {
		switch command {
		case "setup":
			if words[1] == "oracle" {
				return []string{"--autodetect", "--name", "--profile", "--account", "--config", "--region", "--compartment", "--subnet", "--availability-domain", "--image", "--shape", "--ssh-public-key", "--network-only", "help"}
			}
		case "list":
			if words[1] == "recipes" || words[1] == "capabilities" {
				return []string{"--root", "--output", "--output=json", "help"}
			}
		case "recipe":
			if words[1] == "render" {
				return []string{"--name", "--file", "--port", "--inputs", "--include-sensitive", "help"}
			}
			if words[1] == "validate" {
				return []string{"--file", "--output", "--output=json", "help"}
			}
		case "validate":
			if words[1] == "recipe" {
				return []string{"--file", "--output", "--output=json", "help"}
			}
		case "instance":
			return []string{"--access", "--wait", "--timeout", "--yes", "--output", "help"}
		case "network":
			return []string{"--access", "--name", "--cidr", "--subnet-cidr", "--ssh-source", "--wait", "--timeout", "--yes", "--output", "help"}
		case "create":
			if words[1] == "server" {
				return []string{"--name", "--access", "--cpu", "--ram", "--storage-size", "--provider", "--region", "--startup-script", "--port", "--amount", "--yes", "--output", "help"}
			}
		case "app":
			if words[1] == "install" {
				return []string{"--external", "--yes", "--output", "--output=json", "help"}
			}
		}
	}
	switch command {
	case "setup":
		return []string{"oracle", "help"}
	case "unlink":
		return []string{"acc", "access", "help"}
	case "list":
		return []string{"acc", "access", "instances", "networks", "recipes", "capabilities", "help", "--access", "--output", "--output=json"}
	case "recipe":
		return []string{"list", "validate", "render", "help"}
	case "validate":
		return []string{"recipe", "help"}
	case "create":
		return []string{"server", "network", "help"}
	case "run":
		return []string{"--app", "--script", "--recipe", "--specs", "--port", "--name", "--access", "--inputs", "--local", "--dry-run", "--startup-script", "--yes", "--output", "help"}
	case "instance":
		return []string{"get", "start", "stop", "restart", "delete", "help"}
	case "network":
		return []string{"create", "list", "get", "update", "delete", "help"}
	case "oc-hunter":
		return []string{"--access", "--name", "--availability-domains", "--cpu", "--ram", "--storage-size", "--interval", "--attempts", "--timeout", "--state-file", "--dry-run", "--yes", "--output", "help"}
	case "install":
		return []string{"skills", "completion", "help"}
	case "app":
		return []string{"install", "help"}
	case "update":
		return []string{"--version", "--yes", "help"}
	case "plan":
		return []string{"--file", "--output", "help"}
	case "apply":
		return []string{"--file", "--hash", "--yes", "--output", "help"}
	case "completion":
		return []string{"bash", "zsh", "fish", "powershell"}
	default:
		return topLevelCommands
	}
}

func completionScript(shell string) (string, bool) {
	switch strings.ToLower(shell) {
	case "bash":
		return `# Infinity-IaC Bash completion
_infinity_iac_complete() {
  local current="${COMP_WORDS[COMP_CWORD]}"
  local candidates
  candidates=$(infinity-iac __complete -- "${COMP_WORDS[@]}") || return 0
  COMPREPLY=( $(compgen -W "$candidates" -- "$current") )
}
complete -F _infinity_iac_complete infinity-iac
`, true
	case "zsh":
		return `# Infinity-IaC Zsh completion
_infinity_iac() {
  local -a candidates
  candidates=(${(f)"$(infinity-iac __complete -- ${words[@]})"})
  _describe 'infinity-iac' candidates
}
compdef _infinity_iac infinity-iac
`, true
	case "fish":
		return `# Infinity-IaC Fish completion
complete -c infinity-iac -f -a '(infinity-iac __complete -- (commandline -opc))'
`, true
	case "powershell", "pwsh":
		return `# Infinity-IaC PowerShell completion
Register-ArgumentCompleter -Native -CommandName infinity-iac -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $elements = @($commandAst.CommandElements | ForEach-Object { $_.ToString() })
  infinity-iac __complete -- @elements | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
  }
}
`, true
	default:
		return "", false
	}
}
