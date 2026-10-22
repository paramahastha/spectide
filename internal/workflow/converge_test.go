package workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestRunConvergenceImplementsOnlyAppendedTasksThenRepeatsTests(t *testing.T) {
	ledger := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	gates := map[string]string{"tests": "FAIL"}
	convergeCalls, implementCalls, testCalls := 0, 0, 0
	hooks := ConvergenceHooks{
		Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
			return ConvergenceSnapshot{TaskLedger: ledger, GateVerdicts: gates}, nil
		},
		Converge: func(context.Context) (string, error) {
			convergeCalls++
			if convergeCalls == 1 {
				ledger += "\n## Phase 2: Convergence\n- [ ] T002 Add missing regression test\n"
				return "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T002 Add missing regression test\n", nil
			}
			return "✅ Converged — required gates pass.", nil
		},
		Implement: func(_ context.Context, tasks []string) error {
			implementCalls++
			if len(tasks) != 1 || !strings.Contains(tasks[0], "T002 Add missing regression test") {
				t.Fatalf("implementation received non-appended tasks: %v", tasks)
			}
			return nil
		},
		Test: func(context.Context) (map[string]string, error) {
			testCalls++
			gates = map[string]string{"tests": "PASS"}
			return map[string]string{"required_tests": "PASS"}, nil
		},
	}
	converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits())
	if err != nil || !converged || convergeCalls != 2 || implementCalls != 1 || testCalls != 1 {
		t.Fatalf("converged=%v err=%v converge=%d implement=%d tests=%d", converged, err, convergeCalls, implementCalls, testCalls)
	}
}

func TestRunConvergenceRejectsConvergedReportWithFailingRequiredGate(t *testing.T) {
	ledger := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	hooks := ConvergenceHooks{
		Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
			return ConvergenceSnapshot{TaskLedger: ledger, GateVerdicts: map[string]string{"tests": "FAIL"}}, nil
		},
		Converge:  func(context.Context) (string, error) { return "✅ Converged — required gates pass.", nil },
		Implement: func(context.Context, []string) error { return nil },
		Test:      func(context.Context) (map[string]string, error) { return map[string]string{"tests": "PASS"}, nil },
	}
	if converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits()); converged || err == nil || !strings.Contains(err.Error(), "CONVERGENCE_REQUIRED_GATE_FAILED") {
		t.Fatalf("converged=%v err=%v", converged, err)
	}
}

func TestRunConvergenceRejectsReportedTasksThatDifferFromLedger(t *testing.T) {
	ledger := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	hooks := ConvergenceHooks{
		Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
			return ConvergenceSnapshot{TaskLedger: ledger, GateVerdicts: map[string]string{"tests": "FAIL"}}, nil
		},
		Converge: func(context.Context) (string, error) {
			ledger += "\n## Phase 2: Convergence\n- [ ] T002 Persisted task\n"
			return "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T003 Different reported task\n", nil
		},
		Implement: func(context.Context, []string) error { t.Fatal("implemented mismatched report tasks"); return nil },
		Test: func(context.Context) (map[string]string, error) {
			t.Fatal("tested after report mismatch")
			return nil, nil
		},
	}
	if converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits()); converged || err == nil || !strings.Contains(err.Error(), "CONVERGENCE_REPORT_TASKS_MISMATCH") {
		t.Fatalf("converged=%v err=%v", converged, err)
	}
}

func TestRunConvergenceRejectsNoProgressAndTaskRewrite(t *testing.T) {
	old := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	t.Run("no progress", func(t *testing.T) {
		hooks := ConvergenceHooks{
			Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
				return ConvergenceSnapshot{TaskLedger: old, GateVerdicts: map[string]string{"tests": "FAIL"}}, nil
			},
			Converge: func(context.Context) (string, error) {
				return "## Convergence Findings\nOutcome: tasks_appended\n", nil
			},
			Implement: func(context.Context, []string) error { t.Fatal("implemented without progress"); return nil },
			Test:      func(context.Context) (map[string]string, error) { t.Fatal("tested without progress"); return nil, nil },
		}
		if converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits()); converged || err == nil || !strings.Contains(err.Error(), "CONVERGENCE_NO_PROGRESS") {
			t.Fatalf("converged=%v err=%v", converged, err)
		}
	})
	t.Run("rewrite", func(t *testing.T) {
		ledger := old
		hooks := ConvergenceHooks{
			Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
				return ConvergenceSnapshot{TaskLedger: ledger}, nil
			},
			Converge: func(context.Context) (string, error) {
				ledger = "# Replaced Tasks\n\n## Phase 2: Convergence\n- [ ] T002 New task\n"
				return "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T002 New task\n", nil
			},
			Implement: func(context.Context, []string) error { t.Fatal("implemented rewritten tasks"); return nil },
			Test:      func(context.Context) (map[string]string, error) { return map[string]string{"tests": "PASS"}, nil },
		}
		if converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits()); converged || err == nil || !strings.Contains(err.Error(), "CONVERGENCE_NOT_APPEND_ONLY") {
			t.Fatalf("converged=%v err=%v", converged, err)
		}
	})
}

func TestRunConvergenceStopsAfterFiveAppendRounds(t *testing.T) {
	ledger := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	convergeCalls := 0
	var phase, task int
	hooks := ConvergenceHooks{
		Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
			return ConvergenceSnapshot{TaskLedger: ledger}, nil
		},
		Converge: func(context.Context) (string, error) {
			convergeCalls++
			phase++
			task++
			id := task + 1
			ledger += fmt.Sprintf("\n## Phase %d: Convergence\n- [ ] T%03d Appended task\n", phase+1, id)
			return fmt.Sprintf("## Convergence Findings\nOutcome: tasks_appended\n- [ ] T%03d Appended task\n", id), nil
		},
		Implement: func(context.Context, []string) error { return nil },
		Test:      func(context.Context) (map[string]string, error) { return map[string]string{"tests": "PASS"}, nil },
	}
	converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits())
	if converged || err == nil || !strings.Contains(err.Error(), "CONVERGENCE_EXHAUSTED") || convergeCalls != 5 {
		t.Fatalf("converged=%v err=%v convergence calls=%d", converged, err, convergeCalls)
	}
}
