package workflow

import (
	"context"

	"github.com/spectide/spectide/internal/model"
)

func (e *Engine) completeLiveQualityStage(ctx context.Context, runID, stageID, stage, next, configHash, gate, evidence string, budget *LoopBudget, featureDir string) error {
	if err := e.store.Gate(ctx, runID, gate, "PASS", evidence); err != nil {
		return err
	}
	if err := e.store.SetStage(ctx, stageID, model.Succeeded); err != nil {
		return err
	}
	if err := e.store.Event(ctx, runID, stage, "stage.completed", "info", map[string]any{"execution": "LIVE", "gate": gate, "verdict": "PASS", "evidence": evidence}); err != nil {
		return err
	}
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	return e.saveCursorCheckpoint(ctx, run, featureDir, stage, next, configHash, "LIVE", budget.Counts())
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
