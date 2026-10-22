package workflow

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultLoopLimitsMatchFR009(t *testing.T) {
	if got, want := DefaultLoopLimits(), (LoopLimits{AnalyzeRemediations: 3, TestRepairs: 4, ConvergenceRounds: 5}); got != want {
		t.Fatalf("limits=%+v want=%+v", got, want)
	}
	if _, err := NewLoopBudget(LoopLimits{}); err == nil || !strings.Contains(err.Error(), "INVALID_LOOP_LIMITS") {
		t.Fatalf("invalid limits error=%v", err)
	}
}

func TestLoopBudgetAllowsThreeAnalysisRemediationsThenExhausts(t *testing.T) {
	budget, err := NewLoopBudget(DefaultLoopLimits())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := budget.AnalysisRemediation("before", "after"); err != nil {
			t.Fatalf("round %d: %v", i+1, err)
		}
	}
	if err := budget.AnalysisRemediation("before", "after"); err == nil || !strings.Contains(err.Error(), "ANALYZE_REMEDIATION_EXHAUSTED") {
		t.Fatalf("fourth round error=%v", err)
	}
	if err := budget.AnalysisRemediation("same", "same"); err == nil || !strings.Contains(err.Error(), "ANALYZE_REMEDIATION_NO_PROGRESS") {
		t.Fatalf("unchanged artifact error=%v", err)
	}
}

func TestLoopBudgetAllowsFourTestRepairsThenExhausts(t *testing.T) {
	budget, err := NewLoopBudget(DefaultLoopLimits())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := budget.TestRepair(); err != nil {
			t.Fatalf("repair %d: %v", i+1, err)
		}
	}
	if err := budget.TestRepair(); err == nil || !strings.Contains(err.Error(), "TEST_REPAIR_EXHAUSTED") {
		t.Fatalf("fifth repair error=%v", err)
	}
}

func TestSharedRunBudgetAppliesAcrossConvergenceAndFinalReviewStages(t *testing.T) {
	budget, err := NewLoopBudget(DefaultLoopLimits())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < DefaultLoopLimits().ConvergenceRounds; i++ {
		if err := budget.ConvergenceRound("before", "after", nil, map[string]string{"tests": "PASS"}); err != nil {
			t.Fatalf("round %d was rejected: %v", i+1, err)
		}
	}
	hooks := ConvergenceHooks{
		Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
			return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: "tasks", GateVerdicts: map[string]string{"required_tests": "PASS"}}, nil
		},
		Converge:  func(context.Context) (string, error) { return "✅ Converged — required gates pass.", nil },
		Implement: func(context.Context, []string) error { return nil },
		Test: func(context.Context) (map[string]string, error) {
			return map[string]string{"required_tests": "PASS"}, nil
		},
	}
	if _, err := RunConvergenceWithBudget(context.Background(), hooks, budget, DefaultLoopLimits()); err == nil || !strings.Contains(err.Error(), "CONVERGENCE_EXHAUSTED") {
		t.Fatalf("later convergence stage reset the run budget: %v", err)
	}
	review := FinalReviewHooks{Convergence: hooks, Review: func(context.Context) ([]string, error) { return nil, nil }, Fix: func(context.Context, []string) error { return nil }}
	if _, err := RunFinalReviewWithBudget(context.Background(), review, budget, DefaultLoopLimits()); err == nil || !strings.Contains(err.Error(), "CONVERGENCE_EXHAUSTED") {
		t.Fatalf("final-review stage reset the run budget: %v", err)
	}
}

func TestLoopBudgetRequiresProgressAndBoundsFiveConvergenceRounds(t *testing.T) {
	budget, err := NewLoopBudget(DefaultLoopLimits())
	if err != nil {
		t.Fatal(err)
	}
	oldGates := map[string]string{"tests": "FAIL"}
	if err := budget.ConvergenceRound("same", "same", oldGates, map[string]string{"tests": "FAIL"}); err == nil || !strings.Contains(err.Error(), "CONVERGENCE_NO_PROGRESS") {
		t.Fatalf("no-progress convergence error=%v", err)
	}
	for i := 0; i < 5; i++ {
		if err := budget.ConvergenceRound("tasks-before", "tasks-after", oldGates, oldGates); err != nil {
			t.Fatalf("round %d: %v", i+1, err)
		}
	}
	if err := budget.ConvergenceRound("tasks-before", "tasks-after", oldGates, oldGates); err == nil || !strings.Contains(err.Error(), "CONVERGENCE_EXHAUSTED") {
		t.Fatalf("sixth round error=%v", err)
	}
	progressBudget, err := NewLoopBudget(DefaultLoopLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := progressBudget.ConvergenceRound("same", "same", oldGates, map[string]string{"tests": "PASS"}); err != nil {
		t.Fatalf("gate improvement should count as progress: %v", err)
	}
}
