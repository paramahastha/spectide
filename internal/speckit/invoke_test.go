package speckit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
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
