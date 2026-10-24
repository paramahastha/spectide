package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/speckit"
	"github.com/spectide/spectide/internal/store"
)

func TestRestartResumeRequiresAndRestoresVerifiedCheckpoint(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	configurationJSON := fakeRunConfiguration(t)
	run, reused, err := s.CreateRunWithConfiguration(ctx, "", project, "restart safely", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	owner := New(s)
	configuration, err := s.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = s.Event(ctx, run.ID, "analyze", "run.paused", "info", map[string]any{"stage": "analyze"}); err != nil {
		t.Fatal(err)
	}
	route := config.Route{Provider: "fake", Profile: "fake-demo"}
	if err = s.AddStage(ctx, model.Stage{ID: "interrupted-stage-attempt", RunID: run.ID, Key: "analyze", Position: 6, State: model.Pending, Provider: route.Provider, Profile: route.Profile, Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetStage(ctx, "interrupted-stage-attempt", model.StageRun); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: run.ID, Stage: "analyze", Phase: "read_only_analysis", Provider: route.Provider, Profile: route.Profile}); err != nil {
		t.Fatal(err)
	}
	spent := LoopLimits{AnalyzeRemediations: 1, TestRepairs: 2, ConvergenceRounds: 3}
	cursor := checkpointCursor{Phase: "analyze", Operation: "evaluate_analysis_report", Tasks: []string{"T087"}, Check: "unit", Position: 2}
	if err = owner.saveCursorCheckpoint(ctx, run, "", "tasks", "analyze", configuration.SHA256, "SIMULATED", spent, cursor); err != nil {
		t.Fatal(err)
	}
	checkpointRows, err := s.Checkpoints(ctx, run.ID)
	if err != nil || len(checkpointRows) == 0 {
		t.Fatalf("load continuation checkpoint rows=%d err=%v", len(checkpointRows), err)
	}
	var savedCursor cursorCheckpointState
	if err = json.Unmarshal([]byte(checkpointRows[len(checkpointRows)-1].StateJSON), &savedCursor); err != nil {
		t.Fatal(err)
	}
	if savedCursor.LoopCounts != spent || savedCursor.PendingPhase != cursor.Phase || savedCursor.PendingOperation != cursor.Operation || savedCursor.PendingCheck != cursor.Check || savedCursor.TaskPosition != cursor.Position || len(savedCursor.PendingTasks) != 1 || savedCursor.PendingTasks[0] != "T087" {
		t.Fatalf("checkpoint did not persist precise continuation state: %+v", savedCursor)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = New(s).Resume(run.ID); err != nil {
		t.Fatalf("verified restart resume: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resumed, getErr := s.GetRun(ctx, run.ID)
		if getErr == nil && resumed.State == model.Completed {
			events, eventErr := s.Events(ctx, run.ID, 0)
			if eventErr != nil {
				t.Fatal(eventErr)
			}
			for _, event := range events {
				if event.Type == "run.recovery_claimed" {
					resumedStages, stagesErr := s.Stages(ctx, run.ID)
					if stagesErr != nil {
						t.Fatal(stagesErr)
					}
					var prior, replay model.Stage
					for _, stage := range resumedStages {
						if stage.Key == "analyze" && stage.Attempt == 1 {
							prior = stage
						}
						if stage.Key == "analyze" && stage.Attempt == 2 {
							replay = stage
						}
					}
					if prior.State != model.StageFail || replay.State != model.Succeeded {
						t.Fatalf("ambiguous stage attempt was not finalized before numbered replay: prior=%+v replay=%+v", prior, replay)
					}
					attempts, attemptErr := s.ExecutionAttempts(ctx, run.ID)
					if attemptErr != nil || len(attempts) != 1 || attempts[0].Result != "FAILED" || !strings.Contains(attempts[0].Error, "ambiguous") {
						t.Fatalf("ambiguous provider invocation was not finalized honestly: attempts=%+v err=%v", attempts, attemptErr)
					}
					return
				}
			}
			t.Fatal("completed resumed run omitted recovery-claimed evidence")
		}
		time.Sleep(15 * time.Millisecond)
	}
	resumed, _ := s.GetRun(ctx, run.ID)
	t.Fatalf("resumed workflow did not complete, state=%s", resumed.State)
}

func TestRestartResumeBlocksAndFinalizesAmbiguousWriteAttempt(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	state, err := store.Open(filepath.Join(t.TempDir(), "ambiguous-write.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	configurationJSON := fakeRunConfiguration(t)
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "do not replay an ambiguous write", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = state.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = state.Event(ctx, run.ID, "implement", "run.paused", "warning", map[string]any{"stage": "implement"}); err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "interrupted-implementation", RunID: run.ID, Key: "implement", Position: indexOfStage("implement"), State: model.Pending, Provider: "codex", Profile: "fixture", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	if err = state.SetStage(ctx, stage.ID, model.StageRun); err != nil {
		t.Fatal(err)
	}
	if _, err = state.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: run.ID, Stage: "implement", Phase: "implementation", Provider: "codex", Profile: "fixture"}); err != nil {
		t.Fatal(err)
	}
	engine := New(state)
	if err = engine.saveCursorCheckpoint(ctx, run, "", "analyze", "implement", configuration.SHA256, "LIVE", LoopLimits{}, checkpointCursor{Phase: "implement", Operation: "await_sensitive_approval"}); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = engine.Resume(run.ID); err == nil || !strings.Contains(err.Error(), "AMBIGUOUS_WRITE_REQUIRES_REVIEW") {
		t.Fatalf("ambiguous write restart was not blocked: %v", err)
	}
	resumed, err := state.GetRun(ctx, run.ID)
	if err != nil || resumed.State != model.Blocked {
		t.Fatalf("ambiguous write run state=%s err=%v", resumed.State, err)
	}
	stagesAfter, err := state.Stages(ctx, run.ID)
	if err != nil || len(stagesAfter) != 1 || stagesAfter[0].State != model.StageFail {
		t.Fatalf("ambiguous stage was replayed or left active: stages=%+v err=%v", stagesAfter, err)
	}
	attempts, err := state.ExecutionAttempts(ctx, run.ID)
	if err != nil || len(attempts) != 1 || attempts[0].Result != "FAILED" || !strings.Contains(attempts[0].Error, "ambiguous") {
		t.Fatalf("ambiguous provider attempt was not finalized: attempts=%+v err=%v", attempts, err)
	}
}

func TestRestartResumeWaitsForPendingWriteApprovalBeforeProviderAttempt(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	featureDir := filepath.Join(project, "specs", "001-product")
	if err := os.MkdirAll(featureDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"spec.md": "# Product\n", "plan.md": "# Plan\n", "tasks.md": "# Tasks\n"} {
		if err := os.WriteFile(filepath.Join(featureDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commandRel := filepath.Join(".opencode", "commands", "speckit.implement.md")
	commandPath := filepath.Join(project, commandRel)
	if err := os.MkdirAll(filepath.Dir(commandPath), 0700); err != nil {
		t.Fatal(err)
	}
	commandContent := []byte("Implement only the requested task: $ARGUMENTS")
	if err := os.WriteFile(commandPath, commandContent, 0600); err != nil {
		t.Fatal(err)
	}
	commandDigest := sha256.Sum256(commandContent)
	internalDir := filepath.Join(project, "internal")
	if err := os.MkdirAll(internalDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(internalDir, "fixture.go"), []byte("package internal\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "approval-resume.db")
	state, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	capability := verifiedFixtureCapability(t, "codex", "/bin/sh", "")
	profile := model.Profile{Name: "fixture", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{project}, Capability: &capability}
	if _, err = state.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	capability, err = state.SaveProfileCapability(ctx, profile.Name, capability)
	if err != nil {
		t.Fatal(err)
	}
	profile.Capability = &capability
	profileRows, err := state.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	decisions := make(map[string]config.Decision, len(stages))
	for _, stage := range stages {
		decision := config.Decision{Stage: stage, Route: config.Route{Provider: "fake", Profile: "fake-demo"}, Mode: "hybrid", Explanation: "restart fixture"}
		if stage == "implement" {
			decision.Route = config.Route{Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium"}
		}
		decisions[stage] = decision
	}
	effective := buildEffectiveRunConfiguration(cfg, decisions, profileRows)
	configurationJSON, err := json.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "resume after approval", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	if err = state.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "paused-implement", RunID: run.ID, Key: "implement", Position: indexOfStage("implement"), State: model.Pending, Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	if err = state.SetStage(ctx, stage.ID, model.StageRun); err != nil {
		t.Fatal(err)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	owner := New(state)
	owner.discover = func(root string) (speckit.Installation, error) {
		return speckit.Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"implement": "/speckit.implement"}, CommandFiles: map[string]string{"implement": filepath.ToSlash(commandRel)}, CommandHashes: map[string]string{"implement": hex.EncodeToString(commandDigest[:])}}, nil
	}
	if err = owner.saveCursorCheckpoint(ctx, run, featureDir, "analyze", "implement", configuration.SHA256, "LIVE", LoopLimits{}); err != nil {
		t.Fatal(err)
	}
	action := sensitiveWriteAction(workspaceIdentityFingerprint(project), "implement", "implement", profile.Provider, profile.Name, run.Prompt)
	approval, err := state.CreateWorkflowApprovalAndPause(ctx, run.ID, stage.Key, action)
	if err != nil || approval.Status != model.ApprovalPending {
		t.Fatalf("pending approval=%+v err=%v", approval, err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	engine := NewWithAdapters(state, &liveFixtureAdapter{})
	engine.discover = owner.discover
	if err = engine.Resume(run.ID); err != nil {
		t.Fatalf("restart resume while approval is pending: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	resumedIntoApproval := false
	for time.Now().Before(deadline) {
		attempts, attemptsErr := state.ExecutionAttempts(ctx, run.ID)
		if attemptsErr != nil {
			t.Fatal(attemptsErr)
		}
		if len(attempts) > 0 {
			t.Fatalf("provider attempt started before approval: %+v", attempts)
		}
		stagesAfter, stageErr := state.Stages(ctx, run.ID)
		if stageErr != nil {
			t.Fatal(stageErr)
		}
		secondAttempt := false
		for _, item := range stagesAfter {
			if item.Key == "implement" && item.Attempt == 2 && item.State == model.StageRun {
				secondAttempt = true
			}
		}
		events, eventsErr := state.Events(ctx, run.ID, 0)
		if eventsErr != nil {
			t.Fatal(eventsErr)
		}
		pauseCount := 0
		for _, event := range events {
			if event.Type == "run.paused" {
				pauseCount++
			}
		}
		if secondAttempt && pauseCount >= 2 {
			resumedIntoApproval = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !resumedIntoApproval {
		attempts, _ := state.ExecutionAttempts(ctx, run.ID)
		stagesAfter, _ := state.Stages(ctx, run.ID)
		events, _ := state.Events(ctx, run.ID, 0)
		currentRun, _ := state.GetRun(ctx, run.ID)
		t.Fatalf("restarted worker did not return to pending approval: state=%s stages=%+v attempts=%+v events=%+v", currentRun.State, stagesAfter, attempts, events)
	}
	if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalApproved); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		attempts, attemptsErr := state.ExecutionAttempts(ctx, run.ID)
		if attemptsErr != nil {
			t.Fatal(attemptsErr)
		}
		if len(attempts) > 0 && attempts[0].Result != "RUNNING" {
			if len(attempts) != 1 || attempts[0].Stage != "implement" || attempts[0].Result != "SUCCEEDED" {
				t.Fatalf("approved provider attempt=%+v", attempts)
			}
			stagesAfter, stageErr := state.Stages(ctx, run.ID)
			if stageErr != nil {
				t.Fatal(stageErr)
			}
			foundReplay := false
			for _, item := range stagesAfter {
				if item.Key == "implement" && item.Attempt == 2 && item.State == model.Succeeded {
					foundReplay = true
				}
			}
			if !foundReplay {
				t.Fatalf("restart did not append the next stage attempt: %+v", stagesAfter)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("approved provider invocation did not finish")
}

func TestRestartResumeSkipsPersistedLocalQualityCheck(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	featureDir := filepath.Join(project, "specs", "001-product")
	if err := os.MkdirAll(featureDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"spec.md": "# Product\n", "plan.md": "# Plan\n", "tasks.md": "# Tasks\n"} {
		if err := os.WriteFile(filepath.Join(featureDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commandRel := filepath.Join(".opencode", "commands", "speckit.implement.md")
	commandPath := filepath.Join(project, commandRel)
	if err := os.MkdirAll(filepath.Dir(commandPath), 0700); err != nil {
		t.Fatal(err)
	}
	commandContent := []byte("Implement only the requested task: $ARGUMENTS")
	if err := os.WriteFile(commandPath, commandContent, 0600); err != nil {
		t.Fatal(err)
	}
	commandDigest := sha256.Sum256(commandContent)
	for path, content := range map[string]string{
		"go.mod":               "module restartfixture\n\ngo 1.27.2\n",
		"cmd/spectide/main.go": "package main\n\nfunc main() {}\n",
		"internal/fixture.go":  "package internal\n",
	} {
		absolute := filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(project, ".spectide"), 0700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "quality-resume.db")
	state, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	capability := verifiedFixtureCapability(t, "codex", "/bin/sh", "")
	profile := model.Profile{Name: "fixture", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{project}, Capability: &capability}
	if _, err = state.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveProfileCapability(ctx, profile.Name, capability); err != nil {
		t.Fatal(err)
	}
	var configured []effectiveStage
	if err = json.Unmarshal(fakeRunConfiguration(t), &configured); err != nil {
		t.Fatal(err)
	}
	for index := range configured {
		if configured[index].Stage == "implement" {
			configured[index].Decision.Route = config.Route{Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium"}
			configured[index].Simulated = false
		}
	}
	configurationJSON, err := json.Marshal(configured)
	if err != nil {
		t.Fatal(err)
	}
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "resume local quality checks", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	if err = state.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = state.Event(ctx, run.ID, "tests", "run.paused", "info", map[string]any{"stage": "tests"}); err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "interrupted-tests", RunID: run.ID, Key: "tests", Position: indexOfStage("tests"), State: model.StageRun, Provider: "local", Profile: "engine", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Second)
	qualityExecution, err := state.SaveQualityExecution(ctx, model.QualityExecution{
		RunID: run.ID, Phase: "tests", ProcessID: os.Getpid(), StartedAt: started, FinishedAt: started.Add(time.Second), Verdict: "PASS",
		Results: []model.QualityResult{{Key: "format", Verdict: "PASS", Command: []string{"gofmt", "-l", "cmd", "internal"}, ProcessID: os.Getpid(), ExitCode: 0, StartedAt: started}},
	})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewWithAdapters(state, &liveFixtureAdapter{})
	engine.discover = func(root string) (speckit.Installation, error) {
		return speckit.Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"implement": "/speckit.implement"}, CommandFiles: map[string]string{"implement": filepath.ToSlash(commandRel)}, CommandHashes: map[string]string{"implement": hex.EncodeToString(commandDigest[:])}}, nil
	}
	cursor := checkpointCursor{Phase: "tests", Operation: "continue_registered_checks", Check: "format", Position: 1, CompletedQualityExecutionIDs: []string{qualityExecution.ID}}
	if err = engine.saveCursorCheckpoint(ctx, run, featureDir, "implement", "tests", configuration.SHA256, "LOCAL", LoopLimits{}, cursor); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = engine.Resume(run.ID); err != nil {
		t.Fatalf("restart resume at local quality cursor: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		report, reportErr := state.Report(ctx, run.ID)
		if reportErr != nil {
			t.Fatal(reportErr)
		}
		runState, stateErr := state.GetRun(ctx, run.ID)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if runState.State == model.Completed || runState.State == model.Blocked || runState.State == model.Failed {
			counts := map[string]int{}
			for _, execution := range report.QualityExecutions {
				for _, result := range execution.Results {
					counts[result.Key]++
				}
			}
			if counts["format"] != 1 || counts["static_analysis"] != 1 || counts["unit_tests"] != 1 || counts["build"] != 1 {
				t.Fatalf("completed quality checks were replayed or remaining checks were omitted: counts=%v executions=%+v state=%s", counts, report.QualityExecutions, runState.State)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("resumed local quality stage did not finish")
}

func TestRestartResumeImplementsAppendedConvergenceTasksWithoutRepeatingCompletedReport(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	featureDir := filepath.Join(project, "specs", "001-product")
	if err := os.MkdirAll(featureDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"spec.md":  "# Product\n",
		"plan.md":  "# Plan\n\n## Phase 2: Convergence\n- [ ] T999 Add focused regression coverage per FR-029\n",
		"tasks.md": "# Tasks\n\n## Phase 1: Core\n- [x] T001 Existing task\n\n## Phase 2: Convergence\n- [ ] T999 Add focused regression coverage per FR-029\n",
	} {
		if err := os.WriteFile(filepath.Join(featureDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commandFiles := map[string]string{}
	commandHashes := map[string]string{}
	for _, stage := range []string{"implement", "converge", "analyze"} {
		rel := filepath.Join(".opencode", "commands", "speckit."+stage+".md")
		absolute := filepath.Join(project, rel)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		content := []byte("Follow the repository's canonical task scope: $ARGUMENTS")
		if err := os.WriteFile(absolute, content, 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		commandFiles[stage] = filepath.ToSlash(rel)
		commandHashes[stage] = hex.EncodeToString(digest[:])
	}
	for path, content := range map[string]string{
		"go.mod":               "module convergencefixture\n\ngo 1.27.2\n",
		"cmd/spectide/main.go": "package main\n\nfunc main() {}\n",
		"internal/fixture.go":  "package internal\n",
	} {
		absolute := filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(project, ".spectide"), 0700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "convergence-resume.db")
	state, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if state != nil {
			_ = state.Close()
		}
	}()
	capability := verifiedFixtureCapability(t, "codex", "/bin/sh", "")
	profile := model.Profile{Name: "fixture", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{project}, Capability: &capability}
	if _, err = state.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveProfileCapability(ctx, profile.Name, capability); err != nil {
		t.Fatal(err)
	}
	var configured []effectiveStage
	if err = json.Unmarshal(fakeRunConfiguration(t), &configured); err != nil {
		t.Fatal(err)
	}
	for index := range configured {
		if configured[index].Stage == "implement" || configured[index].Stage == "converge" || configured[index].Stage == "final_review" {
			configured[index].Decision.Route = config.Route{Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium"}
			configured[index].Simulated = false
		}
	}
	configurationJSON, err := json.Marshal(configured)
	if err != nil {
		t.Fatal(err)
	}
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "resume appended convergence task", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	if err = state.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = state.Event(ctx, run.ID, "converge", "run.paused", "info", map[string]any{"stage": "converge"}); err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "interrupted-convergence", RunID: run.ID, Key: "converge", Position: indexOfStage("converge"), State: model.StageRun, Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	priorConvergence, err := state.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: run.ID, Stage: "converge", Phase: "convergence", Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	priorConvergence.Result = "SUCCEEDED"
	priorConvergence.ExitCode = 0
	priorConvergence.StartedAt = time.Now().UTC().Add(-time.Second)
	finished := time.Now().UTC()
	priorConvergence.FinishedAt = &finished
	if err = state.CompleteExecutionAttempt(ctx, priorConvergence); err != nil {
		t.Fatal(err)
	}
	for key, evidence := range map[string]string{"analyze_clean": "read-only analysis passed before convergence", "required_tests": "fixture verified before convergence append"} {
		if err = state.Gate(ctx, run.ID, key, "PASS", evidence); err != nil {
			t.Fatal(err)
		}
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(state)
	if err = engine.saveCursorCheckpoint(ctx, run, featureDir, "tests", "converge", configuration.SHA256, "LIVE", LoopLimits{ConvergenceRounds: 1}, checkpointCursor{Phase: "convergence", Operation: "implement_appended_tasks", Tasks: []string{"T999 Add focused regression coverage per FR-029"}, Position: 1}); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	state = nil
	discovery, err := json.Marshal(struct {
		CommandFiles  map[string]string `json:"command_files"`
		CommandHashes map[string]string `json:"command_hashes"`
	}{CommandFiles: commandFiles, CommandHashes: commandHashes})
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestRestartResumeHelperProcess$")
	child.Env = append(os.Environ(), "SPECTIDE_RESTART_HELPER=1", "SPECTIDE_RESTART_DB="+dbPath, "SPECTIDE_RESTART_RUN="+run.ID, "SPECTIDE_RESTART_DISCOVERY="+string(discovery))
	childOutput, outputErr := child.StdoutPipe()
	if outputErr != nil {
		t.Fatal(outputErr)
	}
	var childStderr strings.Builder
	child.Stderr = &childStderr
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	state, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	approved := false
	for time.Now().Before(deadline) {
		approvals, stateErr := state.Approvals(ctx, run.ID)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		for _, pending := range approvals {
			if pending.Status != model.ApprovalPending {
				continue
			}
			attemptsBeforeApproval, attemptsErr := state.ExecutionAttempts(ctx, run.ID)
			if attemptsErr != nil {
				t.Fatal(attemptsErr)
			}
			for _, attempt := range attemptsBeforeApproval {
				if attempt.Stage == "implement" && attempt.Phase == "convergence_implementation" {
					t.Fatalf("provider attempt began before nested approval: %+v", attempt)
				}
			}
			if err = state.ResolveApprovalForRun(ctx, run.ID, pending.ID, model.ApprovalApproved); err != nil {
				t.Fatal(err)
			}
			approved = true
			break
		}
		if approved {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !approved {
		_ = child.Process.Kill()
		_, _ = io.ReadAll(childOutput)
		_ = child.Wait()
		t.Fatalf("nested convergence approval did not become pending: %s", childStderr.String())
	}
	output, readErr := io.ReadAll(childOutput)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err = child.Wait(); err != nil {
		t.Fatalf("separate-process resume failed: %v: %s%s", err, output, childStderr.String())
	}
	{
		runState, stateErr := state.GetRun(ctx, run.ID)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if runState.State != model.Completed {
			t.Fatalf("separate-process resumed state=%s output=%s stderr=%s", runState.State, output, childStderr.String())
		}
		attempts, attemptsErr := state.ExecutionAttempts(ctx, run.ID)
		if attemptsErr != nil {
			t.Fatal(attemptsErr)
		}
		counts := map[string]int{}
		for _, attempt := range attempts {
			counts[attempt.Stage+"/"+attempt.Phase]++
		}
		if runState.State != model.Completed || counts["converge/convergence"] != 2 || counts["implement/convergence_implementation"] != 1 {
			t.Fatalf("restart repeated or skipped convergence work: counts=%v attempts=%+v state=%s", counts, attempts, runState.State)
		}
		firstConvergenceSequence, implementationSequence, nextConvergenceSequence := int64(0), int64(0), int64(0)
		for _, attempt := range attempts {
			switch {
			case attempt.Stage == "converge" && attempt.Phase == "convergence" && firstConvergenceSequence == 0:
				firstConvergenceSequence = attempt.StartEventSeq
			case attempt.Stage == "implement" && attempt.Phase == "convergence_implementation":
				implementationSequence = attempt.StartEventSeq
			case attempt.Stage == "converge" && attempt.Phase == "convergence":
				nextConvergenceSequence = attempt.StartEventSeq
			}
		}
		if !(firstConvergenceSequence > 0 && firstConvergenceSequence < implementationSequence && implementationSequence < nextConvergenceSequence) {
			t.Fatalf("convergence/implementation attempt ordering is invalid: attempts=%+v", attempts)
		}
		report, reportErr := state.Report(ctx, run.ID)
		if reportErr != nil {
			t.Fatal(reportErr)
		}
		qualityBetweenImplementationAndConvergence := false
		for _, execution := range report.QualityExecutions {
			if execution.Phase == "convergence" && execution.Verdict == "PASS" && execution.CompletionEventSeq > implementationSequence && execution.CompletionEventSeq < nextConvergenceSequence {
				qualityBetweenImplementationAndConvergence = true
			}
		}
		if !qualityBetweenImplementationAndConvergence {
			t.Fatalf("fresh required checks did not pass between implementation and reconvergence: attempts=%+v quality=%+v", attempts, report.QualityExecutions)
		}
		checkpoint, cursor, checkpointErr := New(state).latestCursorCheckpoint(ctx, run.ID)
		if checkpointErr != nil || cursor.LoopCounts.ConvergenceRounds < 1 {
			t.Fatalf("spent convergence budget was not preserved after process restart: cursor=%+v checkpoint=%+v err=%v", cursor, checkpoint, checkpointErr)
		}
		gates, gatesErr := state.Gates(ctx, run.ID)
		if gatesErr != nil || gateByKey(gates, "required_tests").Verdict != "PASS" || gateByKey(gates, "converged").Verdict != "PASS" || gateByKey(gates, "final_review").Verdict != "PASS" {
			t.Fatalf("recovery terminal gates are incomplete: gates=%+v err=%v", gates, gatesErr)
		}
	}
}

func TestRestartResumeReplaysApprovedTestRepairWithoutDoubleCharging(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	featureDir := filepath.Join(project, "specs", "001-product")
	if err := os.MkdirAll(featureDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"spec.md": "# Product\n", "plan.md": "# Plan\n", "tasks.md": "# Tasks\n"} {
		if err := os.WriteFile(filepath.Join(featureDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commandRel := filepath.Join(".opencode", "commands", "speckit.implement.md")
	commandPath := filepath.Join(project, commandRel)
	if err := os.MkdirAll(filepath.Dir(commandPath), 0700); err != nil {
		t.Fatal(err)
	}
	commandContent := []byte("Implement the authorized repair: $ARGUMENTS")
	if err := os.WriteFile(commandPath, commandContent, 0600); err != nil {
		t.Fatal(err)
	}
	commandDigest := sha256.Sum256(commandContent)
	for path, content := range map[string]string{
		"go.mod":                   "module testrepairrestartfixture\n\ngo 1.27.2\n",
		"cmd/spectide/main.go":     "package main\n\nfunc main() {}\n",
		"internal/fixture.go":      "package internal\n",
		"internal/fixture_test.go": "package internal\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestRepairMarker(t *testing.T) {\n\tif _, err := os.Stat(\"repaired.marker\"); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
	} {
		absolute := filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(project, ".spectide"), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(project, "internal", "repaired.marker")
	dbPath := filepath.Join(t.TempDir(), "test-repair-resume.db")
	state, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	capability := verifiedFixtureCapability(t, "codex", "/bin/sh", "")
	profile := model.Profile{Name: "fixture", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{project}, Capability: &capability}
	if _, err = state.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	capability, err = state.SaveProfileCapability(ctx, profile.Name, capability)
	if err != nil {
		t.Fatal(err)
	}
	profile.Capability = &capability
	cfg := config.Default()
	cfg.Execution.TestRepairLimit = 1
	decisions := make(map[string]config.Decision, len(stages))
	for _, stage := range stages {
		route := config.Route{Provider: "fake", Profile: "fake-demo"}
		if stage == "implement" {
			route = config.Route{Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium"}
		}
		decisions[stage] = config.Decision{Stage: stage, Route: route, Mode: "hybrid", Explanation: "fixture"}
	}
	effective := buildEffectiveRunConfiguration(cfg, decisions, []model.Profile{profile})
	configurationJSON, err := json.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "resume an approved test repair", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	if err = state.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = state.Event(ctx, run.ID, "tests", "run.paused", "info", map[string]any{"stage": "tests"}); err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "interrupted-tests-repair", RunID: run.ID, Key: "tests", Position: indexOfStage("tests"), State: model.StageRun, Provider: "local", Profile: "engine", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(state)
	commandFiles := map[string]string{"implement": filepath.ToSlash(commandRel)}
	commandHashes := map[string]string{"implement": hex.EncodeToString(commandDigest[:])}
	if err = engine.saveCursorCheckpoint(ctx, run, featureDir, "implement", "tests", configuration.SHA256, "LIVE", LoopLimits{}, checkpointCursor{Phase: "tests", Operation: "start_registered_checks"}); err != nil {
		t.Fatal(err)
	}
	staleOwner := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, staleOwner, run.ID); err != nil {
		t.Fatal(err)
	}
	discovery, err := json.Marshal(struct {
		CommandFiles  map[string]string `json:"command_files"`
		CommandHashes map[string]string `json:"command_hashes"`
	}{CommandFiles: commandFiles, CommandHashes: commandHashes})
	if err != nil {
		t.Fatal(err)
	}
	startChild := func() *exec.Cmd {
		child := exec.Command(os.Args[0], "-test.run=^TestRestartResumeHelperProcess$")
		child.Env = append(os.Environ(), "SPECTIDE_RESTART_HELPER=1", "SPECTIDE_RESTART_DB="+dbPath, "SPECTIDE_RESTART_RUN="+run.ID, "SPECTIDE_RESTART_DISCOVERY="+string(discovery), "SPECTIDE_RESTART_REPAIR_MARKER="+marker, "SPECTIDE_RESTART_ALLOW_BLOCKED=1")
		return child
	}
	first := startChild()
	var firstOutput bytes.Buffer
	first.Stdout = &firstOutput
	first.Stderr = &firstOutput
	if err = first.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var pending model.Approval
	var firstCheckCount int
	for time.Now().Before(deadline) {
		approvals, stateErr := state.Approvals(ctx, run.ID)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		for _, approval := range approvals {
			if approval.Status == model.ApprovalPending && strings.Contains(approval.Action, "tests_repair") {
				pending = approval
				break
			}
		}
		report, reportErr := state.Report(ctx, run.ID)
		if reportErr != nil {
			t.Fatal(reportErr)
		}
		firstCheckCount = len(report.QualityExecutions)
		currentRun, runErr := state.GetRun(ctx, run.ID)
		if runErr != nil {
			t.Fatal(runErr)
		}
		if currentRun.State == model.Blocked || currentRun.State == model.Failed || currentRun.State == model.Cancelled {
			_ = first.Wait()
			stagesAfter, _ := state.Stages(ctx, run.ID)
			gatesAfter, _ := state.Gates(ctx, run.ID)
			eventsAfter, _ := state.Events(ctx, run.ID, 0)
			t.Fatalf("initial repair restart ended %s before approval: %s stages=%+v gates=%+v events=%+v", currentRun.State, firstOutput.String(), stagesAfter, gatesAfter, eventsAfter)
		}
		if pending.ID != "" && firstCheckCount >= 4 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pending.ID == "" || firstCheckCount < 4 {
		_ = first.Process.Kill()
		_ = first.Wait()
		t.Fatalf("repair did not reach pending approval after registered checks: approval=%+v quality_count=%d output=%s", pending, firstCheckCount, firstOutput.String())
	}
	checkpoint, pendingCursor, err := New(state).latestCursorCheckpoint(ctx, run.ID)
	if err != nil || pendingCursor.ApprovalContinuation == nil || pendingCursor.ApprovalContinuation.Operation != "restart_registered_checks" || pendingCursor.LoopCounts.TestRepairs != 0 {
		_ = first.Process.Kill()
		_ = first.Wait()
		t.Fatalf("pending repair cursor did not preserve the unspent retry slot: cursor=%+v checkpoint=%+v err=%v", pendingCursor, checkpoint, err)
	}
	attempts, err := state.ExecutionAttempts(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, attempt := range attempts {
		if attempt.Stage == "implement" && attempt.Phase == "tests_repair" {
			_ = first.Process.Kill()
			_ = first.Wait()
			t.Fatalf("test repair provider attempt began before approval: %+v", attempt)
		}
	}
	if err = first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = first.Wait()
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = state.ResolveApprovalForRun(ctx, run.ID, pending.ID, model.ApprovalApproved); err != nil {
		t.Fatal(err)
	}
	second := startChild()
	output, err := second.CombinedOutput()
	if err != nil {
		t.Fatalf("restart approved repair in a fresh process: %v: %s", err, output)
	}
	report, err := state.Report(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkCounts := map[string]int{}
	lastVerdicts := map[string]string{}
	for _, execution := range report.QualityExecutions {
		for _, result := range execution.Results {
			checkCounts[result.Key]++
			lastVerdicts[result.Key] = result.Verdict
		}
	}
	for _, key := range []string{"format", "static_analysis", "unit_tests", "build"} {
		if checkCounts[key] != 3 || lastVerdicts[key] != "PASS" {
			currentRun, _ := state.GetRun(ctx, run.ID)
			stagesAfter, _ := state.Stages(ctx, run.ID)
			gatesAfter, _ := state.Gates(ctx, run.ID)
			eventsAfter, _ := state.Events(ctx, run.ID, 0)
			attemptsAfter, _ := state.ExecutionAttempts(ctx, run.ID)
			t.Fatalf("quality checks did not run before and after the restarted repair: counts=%v verdicts=%v state=%s output=%s stages=%+v gates=%+v attempts=%+v events=%+v", checkCounts, lastVerdicts, currentRun.State, output, stagesAfter, gatesAfter, attemptsAfter, eventsAfter)
		}
	}
	attempts, err = state.ExecutionAttempts(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var repair *model.ExecutionAttempt
	for index := range attempts {
		if attempts[index].Stage == "implement" && attempts[index].Phase == "tests_repair" {
			repair = &attempts[index]
		}
	}
	if repair == nil || repair.Result != "SUCCEEDED" {
		t.Fatalf("approved repair was not executed exactly once: attempts=%+v", attempts)
	}
	checkpoint, cursor, err := New(state).latestCursorCheckpoint(ctx, run.ID)
	if err != nil || cursor.LoopCounts.TestRepairs != 1 {
		t.Fatalf("retry budget was not charged exactly once: cursor=%+v checkpoint=%+v err=%v", cursor, checkpoint, err)
	}
}

func TestRestartResumeHelperProcess(t *testing.T) {
	if os.Getenv("SPECTIDE_RESTART_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	var discovery struct {
		CommandFiles  map[string]string `json:"command_files"`
		CommandHashes map[string]string `json:"command_hashes"`
	}
	if err := json.Unmarshal([]byte(os.Getenv("SPECTIDE_RESTART_DISCOVERY")), &discovery); err != nil {
		t.Fatal(err)
	}
	state, err := store.Open(os.Getenv("SPECTIDE_RESTART_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	runID := os.Getenv("SPECTIDE_RESTART_RUN")
	run, err := state.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewWithAdapters(state, &liveFixtureAdapter{repairMarker: os.Getenv("SPECTIDE_RESTART_REPAIR_MARKER")})
	engine.discover = func(root string) (speckit.Installation, error) {
		commands := make(map[string]string, len(discovery.CommandFiles))
		for stage := range discovery.CommandFiles {
			commands[stage] = "/speckit." + stage
		}
		return speckit.Installation{ProjectRoot: root, Verified: true, Commands: commands, CommandFiles: discovery.CommandFiles, CommandHashes: discovery.CommandHashes}, nil
	}
	if err = engine.Resume(runID); err != nil {
		t.Fatalf("fresh-process resume: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resumed, stateErr := state.GetRun(ctx, run.ID)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		switch resumed.State {
		case model.Completed:
			return
		case model.Blocked:
			if os.Getenv("SPECTIDE_RESTART_ALLOW_BLOCKED") == "1" {
				return
			}
			t.Fatalf("fresh-process resume ended %s", resumed.State)
		case model.Failed, model.Cancelled:
			t.Fatalf("fresh-process resume ended %s", resumed.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fresh-process resume did not reach a terminal state")
}

func TestRestartResumeReanalyzesAfterCompletedOwnerRemediation(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	featureDir := filepath.Join(project, "specs", "001-product")
	if err := os.MkdirAll(featureDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"spec.md": "# Product\n", "plan.md": "# Plan\n", "tasks.md": "# Tasks\n"} {
		if err := os.WriteFile(filepath.Join(featureDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commandFiles := map[string]string{}
	commandHashes := map[string]string{}
	for _, stage := range []string{"analyze", "implement"} {
		rel := filepath.Join(".opencode", "commands", "speckit."+stage+".md")
		absolute := filepath.Join(project, rel)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		content := []byte("Use the canonical scope for $ARGUMENTS")
		if err := os.WriteFile(absolute, content, 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		commandFiles[stage] = filepath.ToSlash(rel)
		commandHashes[stage] = hex.EncodeToString(digest[:])
	}
	for path, content := range map[string]string{
		"go.mod":               "module analyzerestartfixture\n\ngo 1.27.2\n",
		"cmd/spectide/main.go": "package main\n\nfunc main() {}\n",
		"internal/fixture.go":  "package internal\n// remediation already applied\n",
	} {
		absolute := filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(project, ".spectide"), 0700); err != nil {
		t.Fatal(err)
	}
	state, err := store.Open(filepath.Join(t.TempDir(), "analyze-resume.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	capability := verifiedFixtureCapability(t, "codex", "/bin/sh", "")
	profile := model.Profile{Name: "fixture", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{project}, Capability: &capability}
	if _, err = state.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveProfileCapability(ctx, profile.Name, capability); err != nil {
		t.Fatal(err)
	}
	var configured []effectiveStage
	if err = json.Unmarshal(fakeRunConfiguration(t), &configured); err != nil {
		t.Fatal(err)
	}
	for index := range configured {
		if configured[index].Stage == "analyze" || configured[index].Stage == "implement" {
			configured[index].Decision.Route = config.Route{Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium"}
			configured[index].Simulated = false
		}
	}
	configurationJSON, err := json.Marshal(configured)
	if err != nil {
		t.Fatal(err)
	}
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "reanalyze after a remediation", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	if err = state.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = state.Event(ctx, run.ID, "analyze", "run.paused", "info", map[string]any{"stage": "analyze"}); err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "interrupted-analyze", RunID: run.ID, Key: "analyze", Position: indexOfStage("analyze"), State: model.StageRun, Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	priorRemediation, err := state.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: run.ID, Stage: "implement", Phase: "analyze_remediation", Provider: "codex", Profile: profile.Name, Model: "fixture/model", Effort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	priorRemediation.Result = "SUCCEEDED"
	priorRemediation.ExitCode = 0
	priorRemediation.StartedAt = time.Now().UTC().Add(-time.Second)
	finished := time.Now().UTC()
	priorRemediation.FinishedAt = &finished
	if err = state.CompleteExecutionAttempt(ctx, priorRemediation); err != nil {
		t.Fatal(err)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewWithAdapters(state, &liveFixtureAdapter{})
	engine.discover = func(root string) (speckit.Installation, error) {
		return speckit.Installation{ProjectRoot: root, Verified: true, Commands: map[string]string{"analyze": "/speckit.analyze", "implement": "/speckit.implement"}, CommandFiles: commandFiles, CommandHashes: commandHashes}, nil
	}
	if err = engine.saveCursorCheckpoint(ctx, run, featureDir, "tasks", "analyze", configuration.SHA256, "LIVE", LoopLimits{AnalyzeRemediations: 1}, checkpointCursor{Phase: "analyze", Operation: "continue_after_remediation"}); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	stopApproval := approveWhenRequested(t, state, run.ID)
	defer stopApproval()
	if err = engine.Resume(run.ID); err != nil {
		t.Fatalf("restart resume after analyze remediation: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		runState, stateErr := state.GetRun(ctx, run.ID)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if runState.State == model.Completed || runState.State == model.Blocked || runState.State == model.Failed {
			attempts, attemptsErr := state.ExecutionAttempts(ctx, run.ID)
			if attemptsErr != nil {
				t.Fatal(attemptsErr)
			}
			remediationCount, analyzeCount := 0, 0
			for _, attempt := range attempts {
				if attempt.Stage == "implement" && attempt.Phase == "analyze_remediation" {
					remediationCount++
				}
				if attempt.Stage == "analyze" {
					analyzeCount++
				}
			}
			stagesAfter, stageErr := state.Stages(ctx, run.ID)
			if stageErr != nil {
				t.Fatal(stageErr)
			}
			analyzeReplay := false
			for _, item := range stagesAfter {
				if item.Key == "analyze" && item.Attempt == 2 && item.State == model.Succeeded {
					analyzeReplay = true
				}
			}
			if remediationCount != 1 || analyzeCount != 1 || !analyzeReplay {
				t.Fatalf("analyze remediation replayed or read-only analysis omitted: attempts=%+v stages=%+v state=%s", attempts, stagesAfter, runState.State)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	current, _ := state.GetRun(ctx, run.ID)
	attempts, _ := state.ExecutionAttempts(ctx, run.ID)
	stagesAfter, _ := state.Stages(ctx, run.ID)
	events, _ := state.Events(ctx, run.ID, 0)
	t.Fatalf("resumed analyze stage did not finish: state=%s attempts=%+v stages=%+v events=%+v", current.State, attempts, stagesAfter, events)
}

func TestCheckpointTracksLatestArtifactVersionsAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	feature := filepath.Join(project, "specs", "001-product")
	if err := os.MkdirAll(feature, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
		if err := os.WriteFile(filepath.Join(feature, name), []byte("final "+name+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sourcePath := filepath.Join(project, "source.go")
	if err := os.WriteFile(sourcePath, []byte("package source // final\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, _, err := s.CreateRunWithConfiguration(ctx, "", project, "checkpoint successive artifact revisions", fakeRunConfiguration(t))
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"specs/001-product/spec.md", "specs/001-product/tasks.md", "source.go"}
	baseTime := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	for _, path := range paths {
		oldContent := []byte("historical " + path + "\n")
		currentPath := filepath.Join(project, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(currentPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(currentPath, oldContent, 0600); err != nil {
			t.Fatal(err)
		}
		oldDigest := sha256.Sum256(oldContent)
		if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "specify", Path: path, Kind: "text", SHA256: hex.EncodeToString(oldDigest[:]), ObservedAt: baseTime}); err != nil {
			t.Fatal(err)
		}
		finalContent := []byte("final " + path + "\n")
		if path == "source.go" {
			finalContent = []byte("package source // final\n")
		}
		if err = os.WriteFile(currentPath, finalContent, 0600); err != nil {
			t.Fatal(err)
		}
		finalDigest := sha256.Sum256(finalContent)
		if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "implement", Path: path, Kind: "text", SHA256: hex.EncodeToString(finalDigest[:]), ObservedAt: baseTime.Add(time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	artifacts, err := s.Artifacts(ctx, run.ID)
	if err != nil || len(artifacts) != 6 {
		t.Fatalf("historical artifact observations were not preserved: count=%d err=%v", len(artifacts), err)
	}
	configuration, err := s.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	owner := New(s)
	if err = owner.saveCursorCheckpoint(ctx, run, feature, "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
		t.Fatalf("checkpoint rejected latest artifact revisions: %v", err)
	}
	if _, _, err = owner.verifyRestartCheckpoint(ctx, run, configuration); err != nil {
		t.Fatalf("latest-version checkpoint did not verify for restart: %v", err)
	}
	if err = os.WriteFile(sourcePath, []byte("external drift after checkpoint\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = owner.verifyRestartCheckpoint(ctx, run, configuration); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
		t.Fatalf("genuine post-checkpoint drift was not rejected: %v", err)
	}
}

func TestCheckpointAcceptsObservedArtifactRemovalAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	feature := filepath.Join(project, "specs", "001-product")
	if err := os.MkdirAll(feature, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
		if err := os.WriteFile(filepath.Join(feature, name), []byte("# Historical "+name+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, _, err := s.CreateRunWithConfiguration(ctx, "", project, "checkpoint an explicit removal", fakeRunConfiguration(t))
	if err != nil {
		t.Fatal(err)
	}
	path := "generated/retired.txt"
	abs := filepath.Join(project, filepath.FromSlash(path))
	if err = os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("historical evidence\n")
	if err = os.WriteFile(abs, content, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	observed := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "implement", Path: path, Kind: "text", SHA256: hex.EncodeToString(digest[:]), ObservedAt: observed}); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(abs); err != nil {
		t.Fatal(err)
	}
	configuration, err := s.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(s)
	if err = engine.saveCursorCheckpoint(ctx, run, feature, "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
		t.Fatalf("unapproved artifact deletion was checkpointed: %v", err)
	}
	if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "implement", Path: path, Kind: "removed", SHA256: removedArtifactSHA256(), ObservedAt: observed.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err = engine.saveCursorCheckpoint(ctx, run, feature, "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
		t.Fatalf("checkpoint rejected explicit removal: %v", err)
	}
	if _, _, err = engine.verifyRestartCheckpoint(ctx, run, configuration); err != nil {
		t.Fatalf("checkpointed removal did not verify for restart: %v", err)
	}
	artifacts, err := s.Artifacts(ctx, run.ID)
	if err != nil || len(artifacts) != 2 || artifacts[0].SHA256 != hex.EncodeToString(digest[:]) || artifacts[1].Kind != "removed" {
		t.Fatalf("artifact history was not preserved: artifacts=%+v err=%v", artifacts, err)
	}
	if err = os.WriteFile(abs, []byte("unapproved recreation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = engine.verifyRestartCheckpoint(ctx, run, configuration); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
		t.Fatalf("artifact recreation after checkpoint was accepted: %v", err)
	}
}

func TestCLISubprocessResumeWaitsForDurableTerminalEvidence(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	dbPath := filepath.Join(t.TempDir(), "state.db")
	state, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "resume in a foreground CLI", fakeRunConfiguration(t))
	if err != nil || reused {
		t.Fatalf("run=%+v reused=%v err=%v", run, reused, err)
	}
	if err = state.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = state.Event(ctx, run.ID, stages[0], "run.paused", "info", map[string]any{"stage": stages[0]}); err != nil {
		t.Fatal(err)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = New(state).saveCursorCheckpoint(ctx, run, "", "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(t.TempDir(), "spectide")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/spectide")
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		t.Fatalf("build CLI: %v: %s", buildErr, output)
	}
	started := time.Now()
	command := exec.Command(binary, "--db", dbPath, "resume", run.ID)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resume CLI: %v: %s", err, output)
	}
	if elapsed := time.Since(started); elapsed < 500*time.Millisecond {
		t.Fatalf("resume CLI exited before foreground work completed (%s): %s", elapsed, output)
	}

	state, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	completed, err := state.GetRun(ctx, run.ID)
	if err != nil || completed.State != model.Completed {
		t.Fatalf("resumed run=%+v err=%v", completed, err)
	}
	report, err := state.Report(ctx, run.ID)
	if err != nil || report.Acceptance.Verdict != "SIMULATED" {
		t.Fatalf("terminal report acceptance=%+v err=%v", report.Acceptance, err)
	}
	events, err := state.Events(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	claimed, terminal := false, false
	for _, event := range events {
		if event.Type == "run.recovery_claimed" {
			claimed = true
		}
		if event.Type == "run.COMPLETED" {
			terminal = true
		}
	}
	if !claimed || !terminal {
		t.Fatalf("resumed terminal events missing: recovery=%v completed=%v", claimed, terminal)
	}
}

func TestRestartResumeBlocksOnManagedWorkspaceDriftWithoutOverwriting(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	configurationJSON := fakeRunConfiguration(t)
	run, _, err := s.CreateRunWithConfiguration(ctx, "", project, "preserve drift evidence", configurationJSON)
	if err != nil {
		t.Fatal(err)
	}
	owner := New(s)
	configuration, err := s.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = s.Event(ctx, run.ID, stages[0], "run.paused", "info", map[string]any{"stage": stages[0]}); err != nil {
		t.Fatal(err)
	}
	if err = owner.saveCursorCheckpoint(ctx, run, "", "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(project, "managed.txt")
	if err = os.WriteFile(artifact, []byte("changed after checkpoint\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = New(s).Resume(run.ID); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
		t.Fatalf("drifted restart resume err=%v", err)
	}
	got, err := s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Blocked {
		t.Fatalf("drifted run=%+v err=%v", got, err)
	}
	content, err := os.ReadFile(artifact)
	if err != nil || string(content) != "changed after checkpoint\n" {
		t.Fatalf("recovery overwrote drift evidence %q err=%v", content, err)
	}
}

func TestRestartResumeBlocksOnRegisteredArtifactAndGitHeadDrift(t *testing.T) {
	for _, drift := range []string{"registered artifact content", "Git HEAD", "checkpoint identity", "immutable configuration hash"} {
		t.Run(drift, func(t *testing.T) {
			ctx := context.Background()
			project := initCheckpointGitProject(t)
			s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			run, _, err := s.CreateRunWithConfiguration(ctx, "", project, "preserve registered evidence", fakeRunConfiguration(t))
			if err != nil {
				t.Fatal(err)
			}
			owner := New(s)
			configuration, err := s.RunConfiguration(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
				t.Fatal(err)
			}
			if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
				t.Fatal(err)
			}
			if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
				t.Fatal(err)
			}
			if err = s.Event(ctx, run.ID, stages[0], "run.paused", "info", map[string]any{"stage": stages[0]}); err != nil {
				t.Fatal(err)
			}
			artifactPath := filepath.Join(project, "generated", "evidence.txt")
			if drift == "registered artifact content" {
				if err = os.MkdirAll(filepath.Dir(artifactPath), 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(artifactPath, []byte("registered content\n"), 0600); err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256([]byte("registered content\n"))
				if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "specify", Path: "generated/evidence.txt", Kind: "text", SHA256: hex.EncodeToString(digest[:])}); err != nil {
					t.Fatal(err)
				}
			}
			if err = owner.saveCursorCheckpoint(ctx, run, "", "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
				t.Fatal(err)
			}
			if drift == "registered artifact content" {
				if err = os.WriteFile(artifactPath, []byte("changed artifact content\n"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if drift == "Git HEAD" {
				gitTestCommand(t, project, "commit", "--allow-empty", "-qm", "head drift")
			} else if drift == "checkpoint identity" || drift == "immutable configuration hash" {
				checkpoints, listErr := s.Checkpoints(ctx, run.ID)
				if listErr != nil || len(checkpoints) == 0 {
					t.Fatalf("checkpoint list=%v err=%v", checkpoints, listErr)
				}
				var state map[string]any
				if err = json.Unmarshal([]byte(checkpoints[len(checkpoints)-1].StateJSON), &state); err != nil {
					t.Fatal(err)
				}
				if drift == "checkpoint identity" {
					state["checkpoint_id"] = "different-checkpoint"
				} else {
					state["configuration_sha256"] = strings.Repeat("0", 64)
				}
				encoded, marshalErr := json.Marshal(state)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if _, err = s.DB.ExecContext(ctx, `UPDATE checkpoints SET state_json=? WHERE id=?`, string(encoded), checkpoints[len(checkpoints)-1].ID); err != nil {
					t.Fatal(err)
				}
			}
			stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
			if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
				t.Fatal(err)
			}
			if err = New(s).Resume(run.ID); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
				t.Fatalf("drifted restart resume err=%v", err)
			}
			got, err := s.GetRun(ctx, run.ID)
			if err != nil || got.State != model.Blocked {
				t.Fatalf("drifted run=%+v err=%v", got, err)
			}
			if drift == "registered artifact content" {
				content, readErr := os.ReadFile(artifactPath)
				if readErr != nil || string(content) != "changed artifact content\n" {
					t.Fatalf("recovery overwrote artifact evidence %q err=%v", content, readErr)
				}
			}
		})
	}
}

func fakeRunConfiguration(t *testing.T) []byte {
	t.Helper()
	type configuredStage struct {
		Stage     string          `json:"stage"`
		Decision  config.Decision `json:"decision"`
		Simulated bool            `json:"simulated"`
	}
	var configured []configuredStage
	for _, stage := range stages {
		decision := config.Decision{Stage: stage, Route: config.Route{Provider: "fake", Profile: "fake-demo"}, Mode: "hybrid", Explanation: "fixture"}
		configured = append(configured, configuredStage{Stage: stage, Decision: decision, Simulated: true})
	}
	encoded, err := json.Marshal(configured)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func initCheckpointGitProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	gitTestCommand(t, project, "init", "-q")
	gitTestCommand(t, project, "config", "user.email", "spectide-test@example.invalid")
	gitTestCommand(t, project, "config", "user.name", "SpecTide Test")
	if err := os.WriteFile(filepath.Join(project, ".gitignore"), []byte("generated/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "managed.txt"), []byte("checkpoint content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, project, "add", ".gitignore", "managed.txt")
	gitTestCommand(t, project, "commit", "-qm", "checkpoint fixture")
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func gitTestCommand(t *testing.T, project string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", project}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
