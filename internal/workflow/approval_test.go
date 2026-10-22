package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

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
			action := "Write workspace " + workspaceIdentityFingerprint(project) + " during implement/" + phase + " via codex/fixture"
			approval, err := state.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: action})
			if err != nil {
				t.Fatal(err)
			}
			if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalDenied); err != nil {
				t.Fatal(err)
			}
			engine := New(state)
			if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", phase, project, profile, nil); err == nil || !strings.Contains(err.Error(), "SENSITIVE_ACTION_DENIED") {
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
	action := "Write workspace " + workspaceIdentityFingerprint(project) + " during implement/test_repair via codex/fixture"
	approval, err := state.CreateApproval(ctx, model.Approval{RunID: run.ID, Action: action})
	if err != nil {
		t.Fatal(err)
	}
	if err = state.ResolveApprovalForRun(ctx, run.ID, approval.ID, model.ApprovalApproved); err != nil {
		t.Fatal(err)
	}
	otherScope := t.TempDir()
	for _, deniedAction := range []string{
		"Write workspace " + workspaceIdentityFingerprint(project) + " during implement/final_review_fix via codex/fixture",
		"Write workspace " + workspaceIdentityFingerprint(otherScope) + " during implement/test_repair via codex/fixture",
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
	if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", "test_repair", project, profile, nil); err != nil {
		t.Fatalf("matching approved run/phase/scope was not reused: %v", err)
	}
	if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", "final_review_fix", project, profile, nil); err == nil || !strings.Contains(err.Error(), "SENSITIVE_ACTION_DENIED") {
		t.Fatalf("approval for another phase was reused: %v", err)
	}
	if err = engine.authorizeSensitiveInvocation(ctx, run, "implement", "test_repair", otherScope, profile, nil); err == nil || !strings.Contains(err.Error(), "SENSITIVE_ACTION_DENIED") {
		t.Fatalf("approval for another workspace scope was reused: %v", err)
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
