package speckit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/runner"
)

type fixtureAdapter struct {
	request adapters.Request
}

func (f *fixtureAdapter) Name() adapters.ProviderName { return adapters.Fake }
func (f *fixtureAdapter) Probe(context.Context, adapters.Profile) (adapters.Capability, error) {
	return adapters.Capability{Provider: adapters.Fake}, nil
}
func (f *fixtureAdapter) Build(request adapters.Request) (runner.Invocation, error) {
	f.request = request
	return runner.Invocation{Binary: "/bin/echo", Args: []string{request.Prompt}, Dir: request.ProjectRoot}, nil
}
func (*fixtureAdapter) Normalize([]byte) ([]adapters.Event, error) {
	return []adapters.Event{{Type: "fake.fixture", Summary: "SIMULATED fixture"}}, nil
}

func TestExecuteStageUsesVerifiedPromptAndReadOnlyAnalyzeInvocation(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(".opencode", "commands", "speckit.analyze.md")
	commandPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("Read files without editing. Task: $ARGUMENTS")
	if err := os.WriteFile(commandPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	installation := Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"analyze": "/speckit.analyze"}, CommandFiles: map[string]string{"analyze": filepath.ToSlash(rel)}, CommandHashes: map[string]string{"analyze": hex.EncodeToString(digest[:])}}
	adapter := &fixtureAdapter{}
	result, err := ExecuteStage(context.Background(), installation, adapter, adapters.Request{Profile: adapters.Profile{Provider: adapters.Fake}}, "analyze", "inspect acceptance")
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.request.ReadOnly || adapter.request.ProjectRoot != root || adapter.request.Stage != "analyze" || adapter.request.Prompt != "Read files without editing. Task: inspect acceptance" {
		t.Fatalf("provider request=%+v", adapter.request)
	}
	if result.Process.ExitCode != 0 || len(result.Events) != 1 || result.Events[0].Type != "fake.fixture" {
		t.Fatalf("stage result=%+v", result)
	}
	if err := os.WriteFile(commandPath, []byte("changed after discovery"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteStage(context.Background(), installation, adapter, adapters.Request{Profile: adapters.Profile{Provider: adapters.Fake}}, "analyze", "inspect acceptance"); err == nil {
		t.Fatal("Spec-Kit command changed after discovery but was still executed")
	}
}

func TestClarifyAndImplementPromptRequireExplicitNoopEvidence(t *testing.T) {
	clarify := stageOutcomeInstructions("clarify", "review current spec")
	implement := stageOutcomeInstructions("implement", "complete selected work")
	if !strings.Contains(clarify, "SPECTIDE_NO_CLARIFICATION:") || !strings.Contains(clarify, "make no artifact changes") {
		t.Fatalf("clarify prompt lacks explicit no-op contract: %q", clarify)
	}
	if !strings.Contains(implement, "SPECTIDE_NO_WORK:") || !strings.Contains(implement, "make no workspace changes") {
		t.Fatalf("implement prompt lacks explicit no-op contract: %q", implement)
	}
}

func TestExecuteStageRequiresVerifiedIntegrationAndMatchingProvider(t *testing.T) {
	root := t.TempDir()
	adapter := &fixtureAdapter{}
	request := adapters.Request{Profile: adapters.Profile{Provider: adapters.Fake}}
	if _, err := ExecuteStage(context.Background(), Installation{ProjectRoot: root}, adapter, request, "analyze", ""); err == nil {
		t.Fatal("unverified Spec-Kit installation accepted")
	}
	installation := Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"analyze": "/speckit.analyze"}, CommandFiles: map[string]string{"analyze": filepath.Join(".opencode", "commands", "speckit.analyze.md")}}
	if err := os.MkdirAll(filepath.Join(root, ".opencode", "commands"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, installation.CommandFiles["analyze"]), []byte("review"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.Profile.Provider = adapters.Codex
	if _, err := ExecuteStage(context.Background(), installation, adapter, request, "analyze", ""); err == nil {
		t.Fatal("mismatched profile provider accepted")
	}
}

func TestExecuteStageWithOwnerTemplateAvoidsGeneralImplementationPrompt(t *testing.T) {
	root := t.TempDir()
	commandFiles := map[string]string{
		"implement": filepath.Join(".opencode", "commands", "speckit.implement.md"),
		"plan":      filepath.Join(".opencode", "commands", "speckit.plan.md"),
	}
	for stage, rel := range commandFiles {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		body := "General product implementation instructions. $ARGUMENTS"
		if stage == "plan" {
			body = "Plan command owner-only scope. $ARGUMENTS"
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	installation := Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"implement": "/speckit.implement", "plan": "/speckit.plan"}, CommandFiles: commandFiles, CommandHashes: map[string]string{}}
	for stage, rel := range commandFiles {
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		installation.CommandHashes[stage] = hex.EncodeToString(digest[:])
	}
	adapter := &fixtureAdapter{}
	_, err := ExecuteStageWithTemplate(context.Background(), installation, adapter, adapters.Request{Profile: adapters.Profile{Provider: adapters.Fake}}, "implement", "plan", "Edit only plan.md; do not implement product code.")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.request.Stage != "implement" || adapter.request.ReadOnly || !strings.HasPrefix(adapter.request.Prompt, "Plan command owner-only scope.") || strings.Contains(adapter.request.Prompt, "General product implementation instructions") {
		t.Fatalf("owner remediation received the wrong template or execution policy: %+v", adapter.request)
	}
}

func TestExecuteSpecifyPromptUsesOnlyFreshOrExistingSelectedScope(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "fresh"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			feature := filepath.Join(root, "specs", "001-selected")
			if err := EnsureFeatureDirectory(root, feature); err != nil {
				t.Fatal(err)
			}
			if existing {
				if err := os.WriteFile(filepath.Join(feature, "spec.md"), []byte("canonical product spec\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			rel := filepath.Join(".opencode", "commands", "speckit.specify.md")
			command := filepath.Join(root, rel)
			if err := os.MkdirAll(filepath.Dir(command), 0700); err != nil {
				t.Fatal(err)
			}
			body := []byte("Create one product spec from: $ARGUMENTS")
			if err := os.WriteFile(command, body, 0600); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(body)
			installation := Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"specify": "/speckit.specify"}, CommandFiles: map[string]string{"specify": filepath.ToSlash(rel)}, CommandHashes: map[string]string{"specify": hex.EncodeToString(digest[:])}}
			adapter := &fixtureAdapter{}
			_, err := ExecuteStage(context.Background(), installation, adapter, adapters.Request{FeatureDirectory: feature, Profile: adapters.Profile{Provider: adapters.Fake}}, "specify", "build the selected project")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(adapter.request.Prompt, "Selected canonical feature directory: specs/001-selected") || !strings.Contains(adapter.request.Prompt, "specs/001-selected/spec.md") {
				t.Fatalf("specify request lost selected path: %+v", adapter.request)
			}
		})
	}
}

type streamingFixtureAdapter struct{}

func (streamingFixtureAdapter) Name() adapters.ProviderName { return adapters.OpenCode }
func (streamingFixtureAdapter) Probe(context.Context, adapters.Profile) (adapters.Capability, error) {
	return adapters.Capability{}, nil
}
func (streamingFixtureAdapter) Build(request adapters.Request) (runner.Invocation, error) {
	return runner.Invocation{Binary: "/bin/sh", Args: []string{"-c", `printf '%s\n' '{"type":"text","text":"terminal output"}' '{"type":"step_finish"}'`}, Dir: request.ProjectRoot}, nil
}
func (streamingFixtureAdapter) Normalize(raw []byte) ([]adapters.Event, error) {
	return adapters.NormalizeJSONL(adapters.OpenCode, raw)
}
func (streamingFixtureAdapter) NewOutputStream() *adapters.OutputStream {
	return adapters.NewOutputStream(adapters.OpenCode)
}

func TestExecuteStageParsesStreamingProviderTerminalText(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(".opencode", "commands", "speckit.analyze.md")
	command := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(command), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("Analyze safely: $ARGUMENTS")
	if err := os.WriteFile(command, body, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	installation := Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"analyze": "/speckit.analyze"}, CommandFiles: map[string]string{"analyze": filepath.ToSlash(rel)}, CommandHashes: map[string]string{"analyze": hex.EncodeToString(digest[:])}}
	result, err := ExecuteStage(context.Background(), installation, streamingFixtureAdapter{}, adapters.Request{Profile: adapters.Profile{Provider: adapters.OpenCode}}, "analyze", "review")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "terminal output" || len(result.Events) != 2 || result.Events[0].Type != "opencode.text" || result.Events[1].Type != "opencode.step_finish" {
		t.Fatalf("stream result=%+v", result)
	}
}

type malformedStreamingFixtureAdapter struct{}

func (malformedStreamingFixtureAdapter) Name() adapters.ProviderName { return adapters.OpenCode }
func (malformedStreamingFixtureAdapter) Probe(context.Context, adapters.Profile) (adapters.Capability, error) {
	return adapters.Capability{}, nil
}
func (malformedStreamingFixtureAdapter) Build(request adapters.Request) (runner.Invocation, error) {
	return runner.Invocation{Binary: "/bin/sh", Args: []string{"-c", `printf '%s\n' '{malformed}'`}, Dir: request.ProjectRoot}, nil
}
func (malformedStreamingFixtureAdapter) Normalize([]byte) ([]adapters.Event, error) {
	return nil, nil
}
func (malformedStreamingFixtureAdapter) NewOutputStream() *adapters.OutputStream {
	return adapters.NewOutputStream(adapters.OpenCode)
}

func TestExecuteStagePreservesTypedProviderStreamErrors(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(".opencode", "commands", "speckit.analyze.md")
	command := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(command), 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte("Analyze: $ARGUMENTS")
	if err := os.WriteFile(command, body, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	installation := Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"analyze": "/speckit.analyze"}, CommandFiles: map[string]string{"analyze": filepath.ToSlash(rel)}, CommandHashes: map[string]string{"analyze": hex.EncodeToString(digest[:])}}
	_, err := ExecuteStage(context.Background(), installation, malformedStreamingFixtureAdapter{}, adapters.Request{Profile: adapters.Profile{Provider: adapters.OpenCode}}, "analyze", "inspect")
	var providerErr *adapters.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "MALFORMED_PROVIDER_EVENT" {
		t.Fatalf("typed provider parse error was erased: %v", err)
	}
}

func TestProviderStreamCompletionDistinguishesMissingAndTruncatedTerminal(t *testing.T) {
	missing := adapters.NewProviderError("PROVIDER_TERMINAL_EVENT_MISSING", "missing")
	if got := providerStreamCompletionError(missing, false); got != missing {
		t.Fatalf("non-truncated stream error changed: %v", got)
	}
	var truncated *adapters.ProviderError
	if err := providerStreamCompletionError(missing, true); !errors.As(err, &truncated) || truncated.Code != "PROVIDER_STREAM_TRUNCATED" {
		t.Fatalf("truncated terminal error=%v", err)
	}
	parseErr := adapters.NewProviderError("MALFORMED_PROVIDER_EVENT", "parse")
	if got := providerStreamCompletionError(parseErr, true); got != parseErr {
		t.Fatalf("specific parse error was hidden by truncation: %v", got)
	}
}
