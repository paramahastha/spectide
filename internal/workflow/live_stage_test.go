package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/runner"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/speckit"
	"github.com/spectide/spectide/internal/store"
)

type liveFixtureAdapter struct {
	timeouts        []time.Duration
	models, efforts []string
	analyzeStart    string
	analyzeRelease  string
	analyzeBarrier  sync.Once
}

func approveWhenRequested(t *testing.T, state *store.Store, runID string) func() {
	t.Helper()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				approvals, err := state.Approvals(context.Background(), runID)
				if err != nil {
					t.Errorf("load approvals: %v", err)
					return
				}
				for _, approval := range approvals {
					if approval.Status == model.ApprovalPending {
						if err := state.ResolveApprovalForRun(context.Background(), runID, approval.ID, model.ApprovalApproved); err != nil {
							t.Errorf("approve workflow action: %v", err)
						}
						return
					}
				}
			}
		}
	}()
	return func() { close(stop); <-done }
}

func verifiedFixtureCapability(t *testing.T, provider, executable, configDirectory string) model.ProfileCapability {
	t.Helper()
	reference, err := security.ResolveExecutableReference(executable)
	if err != nil {
		t.Fatal(err)
	}
	approvedConfig, err := security.ResolveConfigDirectory(configDirectory)
	if err != nil {
		t.Fatal(err)
	}
	return model.ProfileCapability{Provider: provider, ExecutableFound: true, ApprovedExecutablePath: reference.Path, ExecutableSHA256: reference.SHA256, ApprovedConfigDirectory: approvedConfig, Version: "fixture-1", Authenticated: "AUTHENTICATED", ModelOverride: true, ModelCatalogStatus: "VERIFIED", ModelValues: []string{"fixture/model"}, EffortOverride: "verified", EffortValues: []string{"medium"}, ReadOnlyExecution: true, WorkspaceWrite: true, SafetyModes: []string{"read-only", "workspace-write"}}
}

func (*liveFixtureAdapter) Name() adapters.ProviderName { return adapters.Codex }
func (*liveFixtureAdapter) Probe(context.Context, adapters.Profile) (adapters.Capability, error) {
	return adapters.Capability{}, nil
}
func (a *liveFixtureAdapter) Build(request adapters.Request) (runner.Invocation, error) {
	a.timeouts = append(a.timeouts, request.Timeout)
	a.models = append(a.models, request.Model)
	a.efforts = append(a.efforts, request.Effort)
	output := "{\"type\":\"message\"}"
	if request.Stage == "plan" {
		path := filepath.Join(request.ProjectRoot, "specs", "001-product", "plan.md")
		return runner.Invocation{Binary: "/bin/sh", Args: []string{"-c", "printf '%s\\n' 'generated plan evidence' > \"" + path + "\"; printf '%s' 'plan generated'"}, Dir: request.ProjectRoot, Timeout: request.Timeout}, nil
	}
	if request.Stage == "implement" {
		path := filepath.Join(request.ProjectRoot, "internal", "fixture.go")
		return runner.Invocation{Binary: "/bin/sh", Args: []string{"-c", "printf '\\n%s\\n' '// implementation evidence' >> \"" + path + "\"; printf '%s' 'implementation generated'"}, Dir: request.ProjectRoot, Timeout: request.Timeout}, nil
	}
	if request.Stage == "analyze" {
		output = "## Specification Analysis Report\n\n| ID | Category | Severity | Location(s) | Summary | Recommendation |\n|---|---|---|---|---|---|\n| — | — | — | — | — | — |\n\n### Metrics\n- Coverage: 100% (1/1)\n"
		barrier := false
		if a.analyzeStart != "" {
			a.analyzeBarrier.Do(func() { barrier = true })
		}
		if barrier {
			script := "touch '" + a.analyzeStart + "'; while [ ! -f '" + a.analyzeRelease + "' ]; do sleep 0.01; done; printf '%s' '" + output + "'"
			return runner.Invocation{Binary: "/bin/sh", Args: []string{"-c", script}, Dir: request.ProjectRoot, Timeout: request.Timeout}, nil
		}
	} else if request.Stage == "converge" {
		output = "✅ Converged — required gates pass."
	}
	return runner.Invocation{Binary: "/bin/sh", Args: []string{"-c", "printf '%s' '" + output + "'"}, Dir: request.ProjectRoot, Timeout: request.Timeout}, nil
}

func TestEngineCompletesVerifiedMixedRunWithApplicableEvidence(t *testing.T) {
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
	constitution := filepath.Join(project, ".specify", "memory", "constitution.md")
	if err = os.MkdirAll(filepath.Dir(constitution), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(constitution, []byte("existing constitution\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The plan owner stage should execute when plan.md is absent; the separate
	// artifact lifecycle tests cover preserving an already-created plan.
	if err = os.Remove(filepath.Join(feature, "plan.md")); err != nil {
		t.Fatal(err)
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
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "fixture@example.invalid"}, {"config", "user.name", "Fixture"}, {"add", "-A"}, {"commit", "-m", "fixture baseline"}} {
		command := exec.Command("git", args...)
		command.Dir = project
		if output, gitErr := command.CombinedOutput(); gitErr != nil {
			t.Fatalf("git %v: %v: %s", args, gitErr, output)
		}
	}
	commandFiles := make(map[string]string)
	commandHashes := make(map[string]string)
	commands := make(map[string]string)
	for _, stage := range []string{"constitution", "specify", "plan", "analyze", "implement", "converge"} {
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
	cfg.Execution.StageTimeoutSeconds = map[string]int{"plan": 42}
	cfg.StageRoutes = map[string]config.Route{
		"constitution": {Provider: "codex", Profile: "fixture"},
		"specify":      {Provider: "codex", Profile: "fixture"},
		"plan":         {Provider: "codex", Profile: "fixture"},
		"analyze":      {Provider: "codex", Profile: "fixture"},
		"implement":    {Provider: "codex", Profile: "fixture"},
		"converge":     {Provider: "codex", Profile: "fixture"},
		"final_review": {Provider: "codex", Profile: "fixture"},
	}
	cfg.StageRoutes["plan"] = config.Route{Provider: "codex", Profile: "fixture", Model: "fixture/model", Effort: "medium"}
	if err = config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveProfile(ctx, model.Profile{Name: "fixture", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{project}}); err != nil {
		t.Fatal(err)
	}
	capability := verifiedFixtureCapability(t, "codex", "/bin/sh", "")
	if _, err = state.SaveProfileCapability(ctx, "fixture", capability); err != nil {
		t.Fatal(err)
	}
	analyzeStarted := filepath.Join(t.TempDir(), "analyze-started")
	analyzeRelease := filepath.Join(t.TempDir(), "analyze-release")
	fixtureAdapter := &liveFixtureAdapter{analyzeStart: analyzeStarted, analyzeRelease: analyzeRelease}
	engine := NewWithAdapters(state, fixtureAdapter)
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
	runIDReady := make(chan string, 1)
	pauseResult := make(chan string, 1)
	go func() {
		id := <-runIDReady
		deadline := time.After(5 * time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		requested := false
		for {
			select {
			case <-deadline:
				if requested {
					pauseResult <- "workflow did not persist inner-loop pause"
				} else {
					pauseResult <- "analyze invocation did not start"
				}
				return
			case <-ticker.C:
				if !requested {
					if _, err := os.Stat(analyzeStarted); err != nil {
						continue
					}
					if err := engine.Pause(id); err != nil {
						pauseResult <- "pause request failed: " + err.Error()
						return
					}
					if err := os.WriteFile(analyzeRelease, []byte("continue"), 0o600); err != nil {
						pauseResult <- err.Error()
						return
					}
					requested = true
					continue
				}
				current, err := state.GetRun(ctx, id)
				if err != nil {
					pauseResult <- err.Error()
					return
				}
				if current.State == model.Paused {
					checkpoints, err := state.Checkpoints(ctx, id)
					if err != nil || len(checkpoints) == 0 {
						pauseResult <- "paused loop has no durable checkpoint"
						return
					}
					var cursor cursorCheckpointState
					if err = json.Unmarshal([]byte(checkpoints[len(checkpoints)-1].StateJSON), &cursor); err != nil || cursor.NextStage != "analyze" {
						pauseResult <- "checkpoint did not retain the active analyze cursor"
						return
					}
					if err = engine.Resume(id); err != nil {
						pauseResult <- "resume failed: " + err.Error()
						return
					}
					pauseResult <- "paused with analyze checkpoint and resumed"
					return
				}
			}
		}
	}()
	runIDReady <- run.ID
	stopApproval := approveWhenRequested(t, state, run.ID)
	defer stopApproval()
	run, err = engine.Wait(ctx, run.ID)
	if result := <-pauseResult; result != "paused with analyze checkpoint and resumed" {
		t.Fatalf("inner-loop control: %s", result)
	}
	if !stringSliceContains(fixtureAdapter.models, "fixture/model") || !stringSliceContains(fixtureAdapter.efforts, "medium") {
		t.Fatalf("verified configured model/effort did not reach offline launch: models=%v efforts=%v", fixtureAdapter.models, fixtureAdapter.efforts)
	}
	if err != nil || run.State != model.Completed {
		t.Fatalf("live workflow run=%+v err=%v", run, err)
	}
	report, err := state.Report(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Acceptance.Verdict != "PASS" || report.Acceptance.Execution != "MIXED" {
		t.Fatalf("mixed-run acceptance=%+v", report.Acceptance)
	}
	if len(report.Checkpoints) == 0 {
		t.Fatal("live feature scope has no checkpoint")
	}
	var checkpointFeature struct {
		FeatureDirectory string `json:"feature_directory"`
	}
	if err = json.Unmarshal([]byte(report.Checkpoints[0].StateJSON), &checkpointFeature); err != nil || checkpointFeature.FeatureDirectory != "specs/001-product" {
		t.Fatalf("selected product scope was not checkpointed: state=%s feature=%q err=%v", report.Checkpoints[0].StateJSON, checkpointFeature.FeatureDirectory, err)
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
	foundConfiguredPlanTimeout := false
	for _, timeout := range fixtureAdapter.timeouts {
		if timeout == 42*time.Second {
			foundConfiguredPlanTimeout = true
			break
		}
	}
	if !foundConfiguredPlanTimeout {
		t.Fatalf("configured plan timeout was not delivered to provider adapter: %v", fixtureAdapter.timeouts)
	}
	for _, stage := range []string{"constitution", "specify"} {
		foundProviderEvent := false
		for _, event := range reportEvents(t, state, run.ID) {
			if event.Type == "provider.codex.message" && event.Stage == stage {
				foundProviderEvent = true
			}
		}
		if foundProviderEvent {
			t.Fatalf("existing %s owner artifact was regenerated", stage)
		}
		foundSkipped := false
		for _, stageReport := range report.Stages {
			if stageReport.Key == stage && stageReport.State == model.Skipped {
				foundSkipped = true
			}
		}
		if !foundSkipped {
			t.Fatalf("existing %s owner artifact was not recorded as preserved", stage)
		}
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
			if gate.Verdict != "NOT_RUN" {
				t.Fatalf("live workflow misreported simulation-only gate: %+v", gate)
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
func (*liveFixtureAdapter) Normalize([]byte) ([]adapters.Event, error) {
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
	if _, err = state.SaveProfile(ctx, model.Profile{Name: "fixture", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{project}}); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveProfileCapability(ctx, "fixture", verifiedFixtureCapability(t, "codex", "/bin/sh", "")); err != nil {
		t.Fatal(err)
	}
	run, _, err := state.CreateRunWithConfiguration(ctx, "", project, "implement a bounded cache", []byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewWithAdapters(state, &liveFixtureAdapter{})
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
	if err != nil || len(events) != 11 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	providerEvent := events[3]
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
	if _, err = state.SaveProfile(ctx, model.Profile{Name: "fixture", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{project}}); err != nil {
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

func TestExecutableReplacementAfterProbeInvalidatesProfileCapability(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	project := t.TempDir()
	executable := filepath.Join(t.TempDir(), "provider")
	if err = os.WriteFile(executable, []byte("#!/bin/sh\necho version one\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	reference, err := security.ResolveExecutableReference(executable)
	if err != nil {
		t.Fatal(err)
	}
	profile := model.Profile{Name: "fixture", Provider: "codex", Executable: executable, AllowedScopes: []string{project}}
	if _, err = state.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	capability := model.ProfileCapability{Provider: "codex", ExecutableFound: true, ApprovedExecutablePath: reference.Path, ExecutableSHA256: reference.SHA256, Authenticated: "AUTHENTICATED"}
	profile.Capability = &capability
	if _, err = state.SaveProfileCapability(ctx, profile.Name, capability); err != nil {
		t.Fatal(err)
	}
	run, _, err := state.CreateRunWithConfiguration(ctx, "", project, "verify identity", []byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	engine := New(state)
	if _, _, err = engine.validateProfileReferences(ctx, run.ID, profile); err != nil {
		t.Fatalf("initial capability reference rejected: %v", err)
	}
	if err = os.WriteFile(executable, []byte("#!/bin/sh\necho replaced provider\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err = engine.validateProfileReferences(ctx, run.ID, profile); err == nil || !strings.Contains(err.Error(), "PROFILE_CAPABILITY_REFERENCE_DRIFT") {
		t.Fatalf("replaced executable was accepted: %v", err)
	}
	profiles, err := state.Profiles(ctx)
	if err != nil || len(profiles) != 1 || profiles[0].Capability != nil {
		t.Fatalf("stale capability was not invalidated: profiles=%+v err=%v", profiles, err)
	}
	events, err := state.Events(ctx, run.ID, 0)
	if err != nil || len(events) == 0 || events[len(events)-1].Type != "profile.capability_invalidated" {
		t.Fatalf("capability invalidation was not reported: events=%+v err=%v", events, err)
	}
}

func stringSliceContains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
