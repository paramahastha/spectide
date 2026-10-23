package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
)

func TestDeniedSensitiveApprovalBlocksBeforeWorkspaceWrite(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	run, err := state.CreateRun(ctx, t.TempDir(), "write project files")
	if err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "implement-attempt", RunID: run.ID, Key: "implement", Position: 0, State: model.Pending, Provider: "codex", Profile: "fixture", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	approval, err := state.CreateWorkflowApprovalAndPause(ctx, run.ID, stage.Key, "Write project files")
	if err != nil {
		t.Fatal(err)
	}
	if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalDenied); err != nil {
		t.Fatal(err)
	}
	current, err := state.Approval(ctx, run.ID, approval.ID)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(state)
	if err = engine.denySensitiveApproval(ctx, run.ID, stage.Key, stage.ID, current); err != nil {
		t.Fatal(err)
	}
	blocked, err := state.GetRun(ctx, run.ID)
	if err != nil || blocked.State != model.Blocked {
		t.Fatalf("run=%+v err=%v", blocked, err)
	}
	stages, err := state.Stages(ctx, run.ID)
	if err != nil || len(stages) != 1 || stages[0].State != model.Skipped {
		t.Fatalf("stages=%+v err=%v", stages, err)
	}
	gates, err := state.Gates(ctx, run.ID)
	if err != nil || len(gates) != 7 {
		t.Fatalf("gates=%+v err=%v", gates, err)
	}
	if gateByKey(gates, "sensitive_action_authorized").Verdict != "BLOCKED" || gateByKey(gates, "workflow_execution").Verdict != "BLOCKED" {
		t.Fatalf("denial and terminal gate verdicts=%+v", gates)
	}
	if _, err = state.DB.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
}

func TestSensitiveWriteApprovalCoversEveryImplementInvocationPhase(t *testing.T) {
	for _, phase := range []string{"implement", "analyze_remediation", "test_repair", "convergence_implementation", "final_review_fix"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			project := t.TempDir()
			state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer state.Close()
			run, err := state.CreateRun(ctx, project, "write files only after approval")
			if err != nil {
				t.Fatal(err)
			}
			stage := model.Stage{ID: "active-outer-stage", RunID: run.ID, Key: "analyze", Position: 6, State: model.Pending, Provider: "codex", Profile: "fixture", Attempt: 1}
			if err = state.AddStage(ctx, stage); err != nil {
				t.Fatal(err)
			}
			if err = state.SetStage(ctx, stage.ID, model.StageRun); err != nil {
				t.Fatal(err)
			}
			if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
				t.Fatal(err)
			}
			profile := model.Profile{Name: "fixture", Provider: "codex"}
			prompt := "Repair only the requested files for " + phase
			action := sensitiveWriteAction(workspaceIdentityFingerprint(project), "implement", phase, profile.Provider, profile.Name, prompt)
			approval, err := state.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: action})
			if err != nil {
				t.Fatal(err)
			}
			if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalDenied); err != nil {
				t.Fatal(err)
			}
			engine := New(state)
			if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", phase, project, profile, prompt, nil); err == nil || !strings.Contains(err.Error(), "SENSITIVE_ACTION_DENIED") {
				t.Fatalf("denied invocation was authorized: %v", err)
			}
			attempts, err := state.ExecutionAttempts(ctx, run.ID)
			if err != nil || len(attempts) != 0 {
				t.Fatalf("provider attempt was created before approval: %+v err=%v", attempts, err)
			}
			gates, err := state.Gates(ctx, run.ID)
			if err != nil || gateByKey(gates, "sensitive_action_authorized").Verdict != "BLOCKED" {
				t.Fatalf("denial gate evidence=%+v err=%v", gates, err)
			}
		})
	}
}

func TestSensitiveWriteApprovalCanReuseOnlyMatchingRunPhaseAndScope(t *testing.T) {
	ctx := context.Background()
	project := t.TempDir()
	state, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	run, err := state.CreateRun(ctx, project, "resume approved write")
	if err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "active-outer-stage", RunID: run.ID, Key: "implement", Position: 7, State: model.Pending, Provider: "codex", Profile: "fixture", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	if err = state.SetStage(ctx, stage.ID, model.StageRun); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	profile := model.Profile{Name: "fixture", Provider: "codex"}
	prompt := "Repair the failed unit check"
	action := sensitiveWriteAction(workspaceIdentityFingerprint(project), "implement", "test_repair", profile.Provider, profile.Name, prompt)
	approval, err := state.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: action})
	if err != nil {
		t.Fatal(err)
	}
	if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalApproved); err != nil {
		t.Fatal(err)
	}
	otherScope := t.TempDir()
	for _, deniedAction := range []string{
		sensitiveWriteAction(workspaceIdentityFingerprint(project), "implement", "final_review_fix", profile.Provider, profile.Name, prompt),
		sensitiveWriteAction(workspaceIdentityFingerprint(project), "implement", "test_repair", profile.Provider, profile.Name, "Repair a different set of files"),
		sensitiveWriteAction(workspaceIdentityFingerprint(otherScope), "implement", "test_repair", profile.Provider, profile.Name, prompt),
	} {
		denied, createErr := state.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: deniedAction})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if err = state.ResolveApprovalForRun(ctx, run.ID, denied.ID, model.ApprovalDenied); err != nil {
			t.Fatal(err)
		}
	}
	engine := New(state)
	if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", "test_repair", project, profile, prompt, nil); err != nil {
		t.Fatalf("matching approved run/phase/scope was not reused: %v", err)
	}
	if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", "test_repair", project, profile, "Repair a different set of files", nil); err == nil || !strings.Contains(err.Error(), "SENSITIVE_ACTION_DENIED") {
		t.Fatalf("approval for a different write intent was reused: %v", err)
	}
	if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", "final_review_fix", project, profile, prompt, nil); err == nil || !strings.Contains(err.Error(), "SENSITIVE_ACTION_DENIED") {
		t.Fatalf("approval for another phase was reused: %v", err)
	}
	if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", "test_repair", otherScope, profile, prompt, nil); err == nil || !strings.Contains(err.Error(), "SENSITIVE_ACTION_DENIED") {
		t.Fatalf("approval for another workspace scope was reused: %v", err)
	}
}

func TestSensitiveApprovalWaitsAndResolvesAfterStoreRestart(t *testing.T) {
	ctx := context.Background()
	project := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	state, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	run, err := state.CreateRun(ctx, project, "resume an approved write")
	if err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "active-implement", RunID: run.ID, Key: "implement", Position: indexOfStage("implement"), State: model.Pending, Provider: "codex", Profile: "fixture", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	if err = state.SetStage(ctx, stage.ID, model.StageRun); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	profile := model.Profile{Name: "fixture", Provider: "codex"}
	prompt := "Apply the approved implementation patch"
	action := sensitiveWriteAction(workspaceIdentityFingerprint(project), "implement", "implement", profile.Provider, profile.Name, prompt)
	approval, err := state.CreateWorkflowApprovalAndPause(ctx, run.ID, stage.Key, action)
	if err != nil || approval.Status != model.ApprovalPending {
		t.Fatalf("approval=%+v err=%v", approval, err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	engine := New(state)
	result := make(chan error, 1)
	go func() {
		result <- engine.authorizeSensitiveInvocation(ctx, run, "implement", "implement", project, profile, prompt, nil)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		attempts, attemptErr := state.ExecutionAttempts(ctx, run.ID)
		if attemptErr != nil || len(attempts) != 0 {
			t.Fatalf("provider invocation appeared before approval: attempts=%+v err=%v", attempts, attemptErr)
		}
		if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalApproved); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err = <-result:
		if err != nil {
			t.Fatalf("persisted pending approval did not resume after restart: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("authorization did not wake after approval resolution")
	}
}

func TestSensitiveApprovalPersistsExecutableStageCursor(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	state, err := store.Open(filepath.Join(t.TempDir(), "approval-cursor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	configurationJSON := fakeRunConfiguration(t)
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "resume an approved write", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	stage := model.Stage{ID: "active-implement-cursor", RunID: run.ID, Key: "implement", Position: indexOfStage("implement"), State: model.Pending, Provider: "codex", Profile: "fixture", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	if err = state.SetStage(ctx, stage.ID, model.StageRun); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	spent := LoopLimits{AnalyzeRemediations: 1, TestRepairs: 2, ConvergenceRounds: 1}
	engine := New(state)
	if err = engine.saveCursorCheckpoint(ctx, run, "", "analyze", "implement", configuration.SHA256, "LIVE", spent); err != nil {
		t.Fatal(err)
	}
	profile := model.Profile{Name: "fixture", Provider: "codex"}
	result := make(chan error, 1)
	go func() {
		result <- engine.authorizeSensitiveInvocation(ctx, run, "implement", "test_repair", project, profile, "Repair the failing local test", nil)
	}()
	var approval model.Approval
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		approvals, loadErr := state.Approvals(ctx, run.ID)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if len(approvals) > 0 && approvals[0].Status == model.ApprovalPending {
			approval = approvals[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if approval.ID == "" {
		t.Fatal("approval did not become pending")
	}
	checkpoints, err := state.Checkpoints(ctx, run.ID)
	if err != nil || len(checkpoints) < 2 {
		t.Fatalf("approval cursor checkpoint missing: checkpoints=%d err=%v", len(checkpoints), err)
	}
	checkpoint, cursor, err := engine.latestCursorCheckpoint(ctx, run.ID)
	if err != nil || checkpoint.Stage != "analyze" || cursor.NextStage != "implement" || cursor.PendingPhase != "test_repair" || cursor.PendingOperation != "await_sensitive_approval" || cursor.LoopCounts != spent {
		t.Fatalf("approval continuation cursor=%+v checkpoint=%+v err=%v", cursor, checkpoint, err)
	}
	if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalApproved); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if err != nil {
			t.Fatalf("approved invocation did not continue: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approved invocation did not wake")
	}
}

func TestDeniedSensitiveApprovalCannotBeBypassedAfterStoreRestart(t *testing.T) {
	ctx := context.Background()
	project := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	state, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	run, err := state.CreateRun(ctx, project, "do not replay a denied write")
	if err != nil {
		t.Fatal(err)
	}
	stage := model.Stage{ID: "active-implement", RunID: run.ID, Key: "implement", Position: indexOfStage("implement"), State: model.Pending, Provider: "codex", Profile: "fixture", Attempt: 1}
	if err = state.AddStage(ctx, stage); err != nil {
		t.Fatal(err)
	}
	if err = state.SetStage(ctx, stage.ID, model.StageRun); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	profile := model.Profile{Name: "fixture", Provider: "codex"}
	prompt := "Apply the denied implementation patch"
	action := sensitiveWriteAction(workspaceIdentityFingerprint(project), "implement", "implement", profile.Provider, profile.Name, prompt)
	approval, err := state.CreateWorkflowApprovalAndPause(ctx, run.ID, stage.Key, action)
	if err != nil || approval.Status != model.ApprovalPending {
		t.Fatalf("approval=%+v err=%v", approval, err)
	}
	if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalDenied); err != nil {
		t.Fatal(err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	engine := New(state)
	if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", "implement", project, profile, prompt, nil); err == nil || !strings.Contains(err.Error(), "SENSITIVE_ACTION_DENIED") {
		t.Fatalf("restart bypassed durable denial: %v", err)
	}
	attempts, err := state.ExecutionAttempts(ctx, run.ID)
	if err != nil || len(attempts) != 0 {
		t.Fatalf("denied write produced provider attempts: %+v err=%v", attempts, err)
	}
}

func gateByKey(gates []model.Gate, key string) model.Gate {
	for _, gate := range gates {
		if gate.Key == key {
			return gate
		}
	}
	return model.Gate{}
}
