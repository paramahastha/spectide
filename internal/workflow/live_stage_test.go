package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/runner"
	"github.com/spectide/spectide/internal/speckit"
	"github.com/spectide/spectide/internal/store"
)

type liveFixtureAdapter struct{}

func (liveFixtureAdapter) Name() adapters.ProviderName { return adapters.Codex }
func (liveFixtureAdapter) Probe(context.Context, adapters.Profile) (adapters.Capability, error) {
	return adapters.Capability{}, nil
}
func (liveFixtureAdapter) Build(request adapters.Request) (runner.Invocation, error) {
	return runner.Invocation{Binary: "/usr/bin/printf", Args: []string{"{\"type\":\"message\"}"}, Dir: request.ProjectRoot}, nil
}

func TestEngineExecutesVerifiedLiveStageAndBlocksSimulatedQualityCompletion(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	project := t.TempDir()
	feature := filepath.Join(project, "specs", "001-product")
	if err = os.MkdirAll(feature, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
		if err = os.WriteFile(filepath.Join(feature, name), []byte(name+" baseline\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(project, "go.mod"), []byte("module fixture\n\ngo 1.27.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mainDir := filepath.Join(project, "cmd", "spectide")
	if err = os.MkdirAll(mainDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(mainDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	internalDir := filepath.Join(project, "internal")
	if err = os.MkdirAll(internalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(internalDir, "fixture.go"), []byte("package internal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rel := ".opencode/commands/speckit.plan.md"
	commandPath := filepath.Join(project, filepath.FromSlash(rel))
	if err = os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
		t.Fatal(err)
	}
	command := []byte("Create a plan for: $ARGUMENTS")
	if err = os.WriteFile(commandPath, command, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(command)
	cfg := config.Default()
	cfg.StageRoutes = map[string]config.Route{"plan": {Provider: "codex", Profile: "fixture"}}
	if err = config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveProfile(ctx, model.Profile{Name: "fixture", Provider: "codex", Executable: "codex", AllowedScopes: []string{project}}); err != nil {
		t.Fatal(err)
	}
	capability := model.ProfileCapability{Provider: "codex", ExecutableFound: true, Version: "fixture-1", Authenticated: "AUTHENTICATED", ReadOnlyExecution: true, WorkspaceWrite: true}
	if _, err = state.SaveProfileCapability(ctx, "fixture", capability); err != nil {
		t.Fatal(err)
	}
	engine := NewWithAdapters(state, liveFixtureAdapter{})
	engine.discover = func(root string) (speckit.Installation, error) {
		return speckit.Installation{
			ProjectRoot: root, Verified: true,
			Commands:      map[string]string{"plan": "/speckit.plan"},
			CommandFiles:  map[string]string{"plan": rel},
			CommandHashes: map[string]string{"plan": hex.EncodeToString(digest[:])},
		}, nil
	}
	run, err := engine.Start(ctx, project, "implement a bounded cache")
	if err != nil {
		t.Fatal(err)
	}
	run, err = engine.Wait(ctx, run.ID)
	if err != nil || run.State != model.Blocked {
		t.Fatalf("live workflow run=%+v err=%v", run, err)
	}
	report, err := state.Report(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var foundLivePlan bool
	for _, event := range reportEvents(t, state, run.ID) {
		if event.Type == "provider.codex.message" && event.Stage == "plan" {
			foundLivePlan = true
		}
	}
	if !foundLivePlan {
		t.Fatal("verified live plan stage produced no persisted provider event")
	}
	if len(report.Gates) < 9 {
		t.Fatalf("live workflow did not persist local quality evidence: %+v", report.Gates)
	}
	var requiredTests string
	var blockedFinalReview bool
	for _, gate := range report.Gates {
		switch gate.Key {
		case "required_tests":
			requiredTests = gate.Verdict
		case "final_review":
			blockedFinalReview = gate.Verdict == "BLOCKED"
		case "analyze_clean", "converged", "simulated_e2e":
			if gate.Verdict != "BLOCKED" {
				t.Fatalf("live workflow claimed simulated quality verdict: %+v", gate)
			}
		}
	}
	if requiredTests != "PASS" {
		t.Fatalf("registered local checks did not pass: %+v", report.Gates)
	}
	if !blockedFinalReview {
		t.Fatalf("incomplete live final review was not blocked: %+v", report.Gates)
	}
}

func reportEvents(t *testing.T, state *store.Store, runID string) []model.Event {
	t.Helper()
	events, err := state.Events(context.Background(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return events
}
func (liveFixtureAdapter) Normalize([]byte) ([]adapters.Event, error) {
	return []adapters.Event{{Type: "codex.message", Summary: "fixture stage completed"}}, nil
}

func TestRunLiveStageUsesVerifiedCommandAndPersistsSanitizedEvidence(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	project := t.TempDir()
	feature := filepath.Join(project, "specs", "001-product")
	if err = os.MkdirAll(feature, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
		if err = os.WriteFile(filepath.Join(feature, name), []byte(name+" baseline\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rel := ".opencode/commands/speckit.plan.md"
	commandPath := filepath.Join(project, filepath.FromSlash(rel))
	if err = os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
		t.Fatal(err)
	}
	command := []byte("Create an implementation plan. Input: $ARGUMENTS")
	if err = os.WriteFile(commandPath, command, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(command)
	installation := speckit.Installation{ProjectRoot: project, Verified: true, Commands: map[string]string{"plan": "/speckit.plan"}, CommandFiles: map[string]string{"plan": rel}, CommandHashes: map[string]string{"plan": hex.EncodeToString(digest[:])}}
	if _, err = state.SaveProfile(ctx, model.Profile{Name: "fixture", Provider: "codex", Executable: "codex", AllowedScopes: []string{project}}); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveProfileCapability(ctx, "fixture", model.ProfileCapability{Provider: "codex", ExecutableFound: true, Version: "fixture-1", Authenticated: "AUTHENTICATED", ReadOnlyExecution: true, WorkspaceWrite: true}); err != nil {
		t.Fatal(err)
	}
	run, _, err := state.CreateRunWithConfiguration(ctx, "", project, "implement a bounded cache", []byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewWithAdapters(state, liveFixtureAdapter{})
	evidence, err := engine.runLiveStage(ctx, run, "plan", config.Route{Provider: "codex", Profile: "fixture"}, installation, feature)
	if err != nil {
		t.Fatal(err)
	}
	if evidence["provider"] != "codex" || evidence["event_count"] != 1 || evidence["execution"] != nil {
		t.Fatalf("unexpected stage evidence: %#v", evidence)
	}
	events, err := state.Events(ctx, run.ID, 0)
	if err != nil || len(events) != 2 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	providerEvent := events[1]
	if providerEvent.Type != "provider.codex.message" || providerEvent.Payload["execution"] != "LIVE" {
		t.Fatalf("unexpected persisted provider event: %+v", providerEvent)
	}
	if _, err = state.SaveProfile(ctx, model.Profile{Name: "fixture", Provider: "codex", Executable: "codex", AllowedScopes: []string{project}}); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.runLiveStage(ctx, run, "plan", config.Route{Provider: "codex", Profile: "fixture"}, installation, feature); err == nil || !strings.Contains(err.Error(), "PROFILE_CAPABILITY_NOT_PROBED") {
		t.Fatalf("profile edit did not invalidate run-time capability evidence: %v", err)
	}
}
