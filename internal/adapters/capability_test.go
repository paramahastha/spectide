package adapters

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
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

func TestProviderOutputStreamHandlesPartialFramesAndExtractsTerminalText(t *testing.T) {
	stream := NewOutputStream(Claude)
	if err := stream.Feed([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"review `)); err != nil {
		t.Fatal(err)
	}
	if err := stream.Feed([]byte(`complete"}]}}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := stream.Feed([]byte(`{"type":"result","result":"review complete"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if got := stream.TerminalText(); got != "review complete" {
		t.Fatalf("terminal=%q", got)
	}
	if events := stream.Events(); len(events) != 2 || events[0].Summary != "structured provider event" {
		t.Fatalf("events=%+v", events)
	}
}

func TestProviderOutputStreamUsesTerminalResultAndRejectsErrors(t *testing.T) {
	stream := NewOutputStream(Codex)
	for _, frame := range []string{`{"type":"item.completed","item":{"type":"agent_message","text":"intermediate"}}`, `{"type":"turn.completed","last_agent_message":"final answer"}`} {
		if err := stream.Feed([]byte(frame + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if got := stream.TerminalText(); got != "final answer" {
		t.Fatalf("terminal=%q", got)
	}
	bad := NewOutputStream(OpenCode)
	if err := bad.Feed([]byte(`{"type":"error","message":"sensitive provider body"}` + "\n")); err == nil {
		t.Fatal("provider error event accepted")
	}
	if err := bad.Close(); err == nil || !strings.Contains(err.Error(), "PROVIDER_REPORTED_ERROR") || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("provider error=%v", err)
	}
	unsupported := NewOutputStream(OpenCode)
	if err := unsupported.Feed([]byte(`{"type":"text","version":"99","text":"hidden"}` + "\n")); err == nil || !strings.Contains(err.Error(), "UNSUPPORTED_PROVIDER_EVENT_VERSION") {
		t.Fatalf("version error=%v", err)
	}
}

func TestProviderOutputStreamRejectsOversizedPartialFrame(t *testing.T) {
	stream := NewOutputStream(Claude)
	if err := stream.Feed([]byte(strings.Repeat("x", maxEventLine+1))); err == nil || !strings.Contains(err.Error(), "PROVIDER_STREAM_LIMIT") {
		t.Fatalf("oversized stream error=%v", err)
	}
}

func TestProviderOutputStreamPublishesEventsDuringFeed(t *testing.T) {
	stream := NewOutputStream(Codex)
	events := make(chan Event, 1)
	stream.SetEventCallback(func(event Event) { events <- event })
	if err := stream.Feed([]byte(`{"type":"item.started","item":{"type":"agent_message"}}` + "\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.Type != "codex.item.started" || event.Summary != "structured provider event" {
			t.Fatalf("stream callback event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("provider event was not published until stream completion")
	}
	if err := stream.Feed([]byte(`{"type":"turn.completed","last_agent_message":"progress"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderOutputStreamPreservesTextDeltaFramingAndRequiresTerminal(t *testing.T) {
	stream := NewOutputStream(Claude)
	for _, frame := range []string{`{"type":"content_block_delta","delta":{"text":"spec"}}`, `{"type":"content_block_delta","delta":{"text":"tide"}}`} {
		if err := stream.Feed([]byte(frame + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if got := stream.TerminalText(); got != "spectide" {
		t.Fatalf("text delta framing changed: %q", got)
	}
	if err := stream.Close(); err == nil || !strings.Contains(err.Error(), "PROVIDER_TERMINAL_EVENT_MISSING") {
		t.Fatalf("missing terminal event error=%v", err)
	}
}

func TestProviderOutputStreamRejectsInvalidVersionShapesAndUnknownTypes(t *testing.T) {
	for _, frame := range []string{
		`{"type":"text","version":1,"text":"x"}`,
		`{"type":"text","version":{"major":1},"text":"x"}`,
	} {
		stream := NewOutputStream(OpenCode)
		if err := stream.Feed([]byte(frame + "\n")); err == nil || !strings.Contains(err.Error(), "UNSUPPORTED_PROVIDER_EVENT_VERSION") {
			t.Errorf("accepted invalid version shape %s: %v", frame, err)
		}
	}
	stream := NewOutputStream(OpenCode)
	if err := stream.Feed([]byte(`{"type":"mystery.event"}` + "\n")); err == nil || !strings.Contains(err.Error(), "UNSUPPORTED_PROVIDER_EVENT") {
		t.Fatalf("unknown provider event type error=%v", err)
	}
}

func TestProviderOutputStreamRetainsRateLimitAndFailureCodes(t *testing.T) {
	for _, testCase := range []struct{ frame, code string }{
		{`{"type":"rate_limits.error"}`, "RATE_LIMITED"},
		{`{"type":"turn.failed"}`, "PROVIDER_REPORTED_ERROR"},
	} {
		stream := NewOutputStream(Codex)
		if err := stream.Feed([]byte(testCase.frame + "\n")); err == nil || !strings.Contains(err.Error(), testCase.code) {
			t.Errorf("%s error=%v", testCase.code, err)
		}
	}
}

func TestProviderOutputStreamRejectsPartialTerminalPayloads(t *testing.T) {
	cases := []struct {
		provider ProviderName
		frame    string
		code     string
	}{
		{Codex, `{"type":"turn.completed"}`, "PROVIDER_EVENT_SHAPE_INVALID"},
		{OpenCode, `{"type":"step_finish"}`, "PROVIDER_TERMINAL_OUTPUT_MISSING"},
	}
	for _, testCase := range cases {
		stream := NewOutputStream(testCase.provider)
		if err := stream.Feed([]byte(testCase.frame + "\n")); err != nil {
			if !strings.Contains(err.Error(), testCase.code) {
				t.Errorf("%s error=%v", testCase.code, err)
			}
			continue
		}
		if err := stream.Close(); err == nil || !strings.Contains(err.Error(), testCase.code) {
			t.Errorf("%s terminal error=%v", testCase.code, err)
		}
	}
}
