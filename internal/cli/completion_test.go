package cli

import (
	"strings"
	"testing"
)

func TestCompletionCandidates(t *testing.T) {
	got := completionCandidates([]string{"create"})
	if !contains(got, "server") || !contains(got, "sandbox") {
		t.Fatalf("create candidates = %v", got)
	}
	if got := completionCandidates(nil); !contains(got, "create") {
		t.Fatalf("top-level candidates = %v", got)
	}
	if got := completionCandidates(nil); !contains(got, "unlink") {
		t.Fatalf("top-level candidates missing unlink = %v", got)
	}
	if got := completionCandidates([]string{"unlink"}); !contains(got, "acc") {
		t.Fatalf("unlink candidates = %v", got)
	}
	if got := completionCandidates([]string{"recipe"}); !contains(got, "list") || !contains(got, "validate") {
		t.Fatalf("recipe candidates = %v", got)
	}
	if got := completionCandidates([]string{"list", "recipes"}); !contains(got, "--root") {
		t.Fatalf("recipe list flags = %v", got)
	}
}

func TestCompletionScriptsCoverSupportedShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		script, ok := completionScript(shell)
		if !ok || !strings.Contains(script, "infinity-iac") {
			t.Fatalf("invalid %s completion script: %q", shell, script)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
