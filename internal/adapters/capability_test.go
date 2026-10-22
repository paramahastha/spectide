package adapters

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCapabilityParsersKeepOnlyVerifiedAnonymizedValues(t *testing.T) {
	if got := claudeAuthStatus(`{"loggedIn":true,"email":"person@example.com","accessToken":"secret"}`, true); got != "AUTHENTICATED" {
		t.Fatalf("claude auth=%q", got)
	}
	if got := claudeAuthStatus(`{"loggedIn":false,"email":"person@example.com"}`, true); got != "UNAUTHENTICATED" {
		t.Fatalf("claude unauth=%q", got)
	}
	if got := textAuthStatus("Not logged in", false); got != "UNAUTHENTICATED" {
		t.Fatalf("codex unauth=%q", got)
	}
	providers := parseOpenCodeAuthProviders(`{"openai":[{"type":"oauth","account":"person@example.com","access":"secret"}],"anthropic":[]}`, true)
	if len(providers) != 1 || providers[0] != "openai" || strings.Contains(strings.Join(providers, " "), "secret") {
		t.Fatalf("providers=%v", providers)
	}
	models := parseModelIDs("openai/gpt-5\nanthropic/sonnet-4", true)
	if len(models) != 2 || models[0] != "anthropic/sonnet-4" || models[1] != "openai/gpt-5" {
		t.Fatalf("models=%v", models)
	}
	efforts := valuesAfterFlag("Options: --effort <level> (low, medium, high, xhigh, max)", "--effort")
	if len(efforts) != 5 || efforts[0] != "high" || efforts[4] != "xhigh" {
		t.Fatalf("efforts=%v", efforts)
	}
	if got := parseModelIDs("openai/gpt-5 token=secret\n", true); len(got) != 1 || got[0] != "openai/gpt-5" {
		t.Fatalf("unsafe model values=%v", got)
	}
}

func TestProbeProviderUsesOnlyStatusAndCapabilityCommands(t *testing.T) {
	path := t.TempDir() + "/claude-fixture"
	script := `#!/bin/sh
case "$*" in
  --version) echo "Claude Code 9.1.0" ;;
  "auth status --json") echo '{"loggedIn":true,"email":"private@example.com","accessToken":"must-not-persist"}' ;;
  --help) echo '--model <model> --effort <level> (low, medium, high, xhigh, max) --permission-mode plan acceptEdits' ;;
  *) echo "unsupported fixture command" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	capability, err := ProbeProvider(context.Background(), Profile{Provider: Claude, Executable: path})
	if err != nil {
		t.Fatal(err)
	}
	if capability.Version != "Claude Code 9.1.0" || capability.Authenticated != "AUTHENTICATED" || !capability.ModelOverride || capability.ModelCatalogStatus != "UNAVAILABLE" || capability.EffortOverride != "verified" || len(capability.EffortValues) != 5 || !capability.ReadOnlyExecution || !capability.WorkspaceWrite {
		t.Fatalf("capability=%+v", capability)
	}
	if strings.Contains(strings.Join(capability.Warnings, " "), "private@example.com") || strings.Contains(strings.Join(capability.CommandCapabilities, " "), "must-not-persist") {
		t.Fatalf("provider output leaked into persisted facts: %+v", capability)
	}
}

func TestUnavailableProviderDiscoveryFailsClosed(t *testing.T) {
	if got := textAuthStatus("", false); got != "UNKNOWN" {
		t.Fatalf("failed auth status=%q", got)
	}
	if got := parseModelIDs("not a model catalog", false); len(got) != 0 {
		t.Fatalf("unavailable catalog=%v", got)
	}
}
