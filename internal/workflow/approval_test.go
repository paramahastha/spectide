package workflow

import (
	"context"
	"path/filepath"
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

func gateByKey(gates []model.Gate, key string) model.Gate {
	for _, gate := range gates {
		if gate.Key == key {
			return gate
		}
	}
	return model.Gate{}
}
