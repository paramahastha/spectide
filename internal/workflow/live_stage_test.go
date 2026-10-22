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
	output := "{\"type\":\"message\"}"
	if request.Stage == "plan" {
		path := filepath.Join(request.ProjectRoot, "specs", "001-product", "plan.md")
		return runner.Invocation{Binary: "/bin/sh", Args: []string{"-c", "printf '%s\\n' 'generated plan evidence' > \"" + path + "\"; printf '%s' 'plan generated'"}, Dir: request.ProjectRoot}, nil
	}
	if request.Stage == "implement" {
		path := filepath.Join(request.ProjectRoot, "internal", "fixture.go")
		return runner.Invocation{Binary: "/bin/sh", Args: []string{"-c", "printf '\\n%s\\n' '// implementation evidence' >> \"" + path + "\"; printf '%s' 'implementation generated'"}, Dir: request.ProjectRoot}, nil
	}
	if request.Stage == "analyze" {
		output = "## Specification Analysis Report\n\n| ID | Category | Severity | Location(s) | Summary | Recommendation |\n|---|---|---|---|---|---|\n| — | — | — | — | — | — |\n\n### Metrics\n- Coverage: 100% (1/1)\n"
	} else if request.Stage == "converge" {
		output = "✅ Converged — required gates pass."
	}
	return runner.Invocation{Binary: "/usr/bin/printf", Args: []string{"%s", output}, Dir: request.ProjectRoot}, nil
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
	commandFiles := make(map[string]string)
	commandHashes := make(map[string]string)
	commands := make(map[string]string)
	for _, stage := range []string{"plan", "analyze", "implement", "converge"} {
		rel := ".opencode/commands/speckit." + stage + ".md"
		commandPath := filepath.Join(project, filepath.FromSlash(rel))
		if err = os.MkdirAll(filepath.Dir(commandPath), 0o700); err != nil {
			t.Fatal(err)
		}
		command := []byte("Run the " + stage + " stage for: $ARGUMENTS")
		if err = os.WriteFile(commandPath, command, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(command)
		commandFiles[stage] = rel
		commandHashes[stage] = hex.EncodeToString(digest[:])
		commands[stage] = "/speckit." + stage
	}
	cfg := config.Default()
	cfg.StageRoutes = map[string]config.Route{
		"plan":         {Provider: "codex", Profile: "fixture"},
		"analyze":      {Provider: "codex", Profile: "fixture"},
		"implement":    {Provider: "codex", Profile: "fixture"},
		"converge":     {Provider: "codex", Profile: "fixture"},
		"final_review": {Provider: "codex", Profile: "fixture"},
	}
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
			Commands: commands, CommandFiles: commandFiles, CommandHashes: commandHashes,
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
	var analyzeClean string
	var converged string
	var finalReview string
	var blockedFinalReview bool
	for _, gate := range report.Gates {
		switch gate.Key {
		case "analyze_clean":
			analyzeClean = gate.Verdict
		case "required_tests":
			requiredTests = gate.Verdict
		case "final_review":
			finalReview = gate.Verdict
			blockedFinalReview = gate.Verdict == "BLOCKED"
		case "converged":
			converged = gate.Verdict
		case "simulated_e2e":
			if gate.Verdict != "BLOCKED" {
				t.Fatalf("live workflow claimed simulated quality verdict: %+v", gate)
			}
		}
	}
	if analyzeClean != "PASS" {
		t.Fatalf("verified live analysis did not pass: %+v", report.Gates)
	}
	if requiredTests != "PASS" {
		t.Fatalf("registered local checks did not pass: %+v", report.Gates)
	}
	if len(report.Attempts) < 5 || len(report.QualityExecutions) < 1 {
		t.Fatalf("provider and test histories were not included in the report: attempts=%d quality=%d", len(report.Attempts), len(report.QualityExecutions))
	}
	phases := map[string]bool{}
	for _, attempt := range report.Attempts {
		if attempt.Result != "SUCCEEDED" || attempt.ProcessID < 1 || attempt.FinishedAt == nil || attempt.StdoutSHA256 == "" {
			t.Fatalf("incomplete provider attempt evidence: %+v", attempt)
		}
		phases[attempt.Phase] = true
	}
	for _, phase := range []string{"analyze", "convergence", "final_review"} {
		if !phases[phase] {
			t.Errorf("missing provider invocation phase %q in %+v", phase, phases)
		}
	}
	for _, execution := range report.QualityExecutions {
		if execution.ProcessID < 1 || execution.FinishedAt.Before(execution.StartedAt) || len(execution.Results) == 0 {
			t.Fatalf("incomplete quality execution evidence: %+v", execution)
		}
		for _, result := range execution.Results {
			if result.ProcessID < 1 {
				t.Fatalf("quality result omitted subprocess identity: %+v", result)
			}
		}
	}
	if converged != "PASS" || finalReview != "PASS" || blockedFinalReview {
		t.Fatalf("live convergence/final review did not pass: %+v", report.Gates)
	}
	var persistedLoopCounts bool
	for _, checkpoint := range report.Checkpoints {
		if checkpoint.Stage == "final_review" && strings.Contains(checkpoint.StateJSON, `"loop_counts"`) {
			persistedLoopCounts = true
		}
	}
	if !persistedLoopCounts {
		t.Fatalf("shared loop budget counts were not persisted in the final-review checkpoint: %+v", report.Checkpoints)
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
	artifactIDs, ok := evidence["artifact_ids"].([]string)
	if !ok || len(artifactIDs) != 1 {
		t.Fatalf("changed plan artifact was not linked to stage evidence: %#v", evidence)
	}
	report, err := state.Report(ctx, run.ID)
	if err != nil || len(report.Artifacts) != 1 || report.Artifacts[0].ID != artifactIDs[0] || report.Artifacts[0].SHA256 == "" {
		t.Fatalf("registered artifact was not exposed in report: artifacts=%+v err=%v", report.Artifacts, err)
	}
	if _, err = engine.runLiveStage(ctx, run, "plan", config.Route{Provider: "codex", Profile: "fixture"}, installation, feature); err == nil || !strings.Contains(err.Error(), "STAGE_EXPECTED_ARTIFACT_MISSING_OR_UNCHANGED") {
		t.Fatalf("unchanged plan output was accepted as stage success: %v", err)
	}
	if _, err = engine.runLiveStagePromptPhase(ctx, run, "implement", "analyze_remediation", config.Route{Provider: "codex", Profile: "fixture"}, "Only edit spec.md", installation, feature, liveStageOptions{templateStage: "plan", ownerPath: "specs/001-product/spec.md"}); err == nil || !strings.Contains(err.Error(), "ANALYZE_REMEDIATION_CHANGED_NON_OWNER_ARTIFACT") {
		t.Fatalf("remediation accepted source-code mutation outside its owning artifact: %v", err)
	}
	events, err := state.Events(ctx, run.ID, 0)
	if err != nil || len(events) != 10 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	providerEvent := events[2]
	if providerEvent.Type != "provider.codex.message" || providerEvent.Payload["execution"] != "LIVE" {
		t.Fatalf("unexpected persisted provider event: %+v", providerEvent)
	}
	attempts, err := state.ExecutionAttempts(ctx, run.ID)
	if err != nil || len(attempts) != 3 || attempts[0].Result != "SUCCEEDED" || attempts[0].Iteration != 1 || attempts[0].ProcessID < 1 || attempts[0].StdoutSHA256 == "" || attempts[1].Iteration != 2 || attempts[1].Result != "FAILED" || attempts[2].Result != "FAILED" || attempts[2].Phase != "analyze_remediation" {
		t.Fatalf("provider invocation history=%+v err=%v", attempts, err)
	}
	if len(attempts[0].ArtifactIDs) != 1 || attempts[0].ArtifactIDs[0] != artifactIDs[0] {
		t.Fatalf("attempt does not link its registered artifact: %+v", attempts[0])
	}
	if _, err = state.SaveProfile(ctx, model.Profile{Name: "fixture", Provider: "codex", Executable: "codex", AllowedScopes: []string{project}}); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.runLiveStage(ctx, run, "plan", config.Route{Provider: "codex", Profile: "fixture"}, installation, feature); err == nil || !strings.Contains(err.Error(), "PROFILE_CAPABILITY_NOT_PROBED") {
		t.Fatalf("profile edit did not invalidate run-time capability evidence: %v", err)
	}
}

func TestLiveStageRequiresChangedArtifactAndRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	feature := filepath.Join(root, "specs", "001-product")
	if err := os.MkdirAll(feature, 0o700); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(feature, "plan.md")
	if err := os.WriteFile(planPath, []byte("old plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := captureStageArtifacts(root, feature, "plan")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = changedStageArtifacts(root, feature, "plan", before); err != nil {
		t.Fatal(err)
	} else if files, _ := changedStageArtifacts(root, feature, "plan", before); len(files) != 0 {
		t.Fatalf("unchanged expected artifact counted as progress: %+v", files)
	}
	if err = os.Remove(planPath); err != nil {
		t.Fatal(err)
	}
	missingBefore, err := captureStageArtifacts(root, feature, "plan")
	if err != nil {
		t.Fatal(err)
	}
	if files, err := changedStageArtifacts(root, feature, "plan", missingBefore); err != nil || len(files) != 0 {
		t.Fatalf("missing expected output was accepted as artifact evidence: files=%+v err=%v", files, err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err = os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, planPath); err != nil {
		t.Fatal(err)
	}
	if _, err = captureStageArtifacts(root, feature, "plan"); err == nil || !strings.Contains(err.Error(), "STAGE_ARTIFACT_PATH_ESCAPE") {
		t.Fatalf("expected artifact symlink escape rejection, got %v", err)
	}
}
