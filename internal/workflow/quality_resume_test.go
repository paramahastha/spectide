package workflow

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/quality"
	"github.com/spectide/spectide/internal/store"
)

func TestRestoreCompletedQualityResultsFromCheckpointCursor(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "quality.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	run, err := state.CreateRun(ctx, t.TempDir(), "resume registered checks")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Second)
	execution, err := state.SaveQualityExecution(ctx, model.QualityExecution{
		RunID: run.ID, Phase: "tests", ProcessID: 1, StartedAt: started, FinishedAt: started.Add(time.Second),
		Verdict: "PASS", Results: []model.QualityResult{{Key: "format", Verdict: "PASS", Command: []string{"gofmt", "-l", "."}, StartedAt: started}},
	})
	if err != nil {
		t.Fatal(err)
	}
	checks := []quality.Check{{Key: "format", Binary: "gofmt"}, {Key: "unit_tests", Binary: "go"}, {Key: "build", Binary: "go"}}
	cursor := cursorCheckpointState{
		PendingPhase: "tests", PendingOperation: "continue_registered_checks", PendingCheck: "format",
		TaskPosition: 1, CompletedQualityExecutionIDs: []string{execution.ID},
	}
	results, err := restoreCompletedQualityResults(ctx, state, run.ID, "tests", checks, cursor)
	if err != nil || len(results) != 1 || results["format"] != "PASS" {
		t.Fatalf("restored results=%v err=%v", results, err)
	}
	registry := quality.NewRegistry(nil)
	for _, check := range checks {
		if err = registry.Register(check); err != nil {
			t.Fatal(err)
		}
	}
	filtered := registry.Except(map[string]struct{}{"format": {}})
	if got := filtered.Checks(); len(got) != 2 || got[0].Key != "unit_tests" || got[1].Key != "build" {
		t.Fatalf("remaining checks were not preserved in order: %+v", got)
	}
	invalid := cursor
	invalid.PendingCheck = "build"
	if _, err = restoreCompletedQualityResults(ctx, state, run.ID, "tests", checks, invalid); err == nil {
		t.Fatal("checkpoint with mismatched completed-check cursor was accepted")
	}
}

func TestTestRepairRestartCountsReuseThePendingRepairBudgetSlot(t *testing.T) {
	counts := LoopLimits{AnalyzeRemediations: 2, TestRepairs: 3, ConvergenceRounds: 4}
	restarted := testRepairRestartCounts(counts)
	if restarted != (LoopLimits{AnalyzeRemediations: 2, TestRepairs: 2, ConvergenceRounds: 4}) {
		t.Fatalf("repair restart counts=%+v", restarted)
	}
	if counts.TestRepairs != 3 {
		t.Fatalf("restart count adjustment mutated the live budget: %+v", counts)
	}
	if zero := testRepairRestartCounts(LoopLimits{}); zero != (LoopLimits{}) {
		t.Fatalf("zero repair count became negative: %+v", zero)
	}
}
