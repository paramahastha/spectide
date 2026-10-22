package workflow

import (
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
