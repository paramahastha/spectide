package workflow

import (
	"context"
	"encoding/json"

	"github.com/spectide/spectide/internal/model"
)

func (e *Engine) completeLiveQualityStage(ctx context.Context, runID, stageID, stage, next, configHash, gate, evidence string, budget *LoopBudget) error {
	if err := e.store.Gate(ctx, runID, gate, "PASS", evidence); err != nil {
		return err
	}
	if err := e.store.SetStage(ctx, stageID, model.Succeeded); err != nil {
		return err
	}
	if err := e.store.Event(ctx, runID, stage, "stage.completed", "info", map[string]any{"execution": "LIVE", "gate": gate, "verdict": "PASS", "evidence": evidence}); err != nil {
		return err
	}
	state, err := json.Marshal(map[string]any{"completed_stage": stage, "next_stage": next, "configuration_sha256": configHash, "cursor_only": true, "execution": "LIVE", "loop_counts": budget.Counts()})
	if err != nil {
		return err
	}
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	checkpoint := model.Checkpoint{RunID: runID, Stage: stage, StateJSON: string(state), WorkspaceFingerprint: workspaceIdentityFingerprint(run.Project)}
	_, err = e.store.SaveCheckpoint(ctx, checkpoint)
	return err
}

func (e *Engine) failLiveQualityStage(ctx context.Context, runID, stage, stageID, gate string, cause error, runCtx context.Context) {
	action, _ := e.store.RunControl(context.Background(), runID)
	if runCtx.Err() != nil || action == "cancel" {
		e.cancelRun(runID, stage, stageID)
		return
	}
	reason := safeStageError(cause)
	_ = e.store.SetStage(context.Background(), stageID, model.StageFail)
	_ = e.store.Gate(context.Background(), runID, gate, "FAIL", reason)
	_ = e.store.Event(context.Background(), runID, stage, stage+".failed", "error", map[string]any{"reason": reason, "execution": "LIVE"})
	_ = e.store.SetRunState(context.Background(), runID, model.Blocked)
}
