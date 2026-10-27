package cli

import (
	"fmt"
	"io"
	"strings"
)

var topLevelCommands = []string{"setup", "list", "create", "run", "network", "instance", "sandbox", "wrap", "install", "plan", "apply", "import", "doctor", "completion", "help"}

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
	switch command {
	case "setup":
		return []string{"oracle", "backup", "help"}
	case "list":
		return []string{"acc", "access", "instances", "recipes", "capabilities", "help", "--output", "--output=json"}
	case "create":
		return []string{"server", "sandbox", "network", "help"}
	case "run":
		return []string{"--app", "--script", "--specs", "--port", "--name", "--yes", "--output", "help"}
	case "instance":
		return []string{"stop", "restart", "delete", "help"}
	case "network":
		return []string{"create", "list", "delete", "help"}
	case "sandbox":
		return []string{"create", "list", "delete", "help"}
	case "wrap":
		return []string{"--script", "--multi", "help"}
	case "install":
		return []string{"skills", "completion", "help"}
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
  candidates=(${(f)"$(infinity-iac __complete -- -- ${words[@]})"})
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
