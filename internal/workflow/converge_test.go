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
			return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: ledger, GateVerdicts: gates}, nil
		},
		Converge: func(context.Context) (string, error) {
			convergeCalls++
			if convergeCalls == 1 {
				ledger += "\n## Phase 2: Convergence\n- [ ] T002 Add missing regression test per FR-029\n"
				return "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T002 Add missing regression test per FR-029\n", nil
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
			return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: ledger, GateVerdicts: map[string]string{"tests": "FAIL"}}, nil
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
			return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: ledger, GateVerdicts: map[string]string{"tests": "FAIL"}}, nil
		},
		Converge: func(context.Context) (string, error) {
			ledger += "\n## Phase 2: Convergence\n- [ ] T002 Persisted task per FR-029\n"
			return "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T003 Different reported task per FR-029\n", nil
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

func TestRunConvergenceRejectsReplacementProductSpecificationPath(t *testing.T) {
	ledger := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	snapshots := 0
	hooks := ConvergenceHooks{
		Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
			snapshots++
			path := "specs/001-build-spectide/spec.md"
			if snapshots > 1 {
				path = "specs/002-replacement/spec.md"
			}
			return ConvergenceSnapshot{SpecificationPath: path, TaskLedger: ledger}, nil
		},
		Converge:  func(context.Context) (string, error) { return "✅ Converged — required gates pass.", nil },
		Implement: func(context.Context, []string) error { return nil },
		Test:      func(context.Context) (map[string]string, error) { return map[string]string{"tests": "PASS"}, nil },
	}
	if converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits()); converged || err == nil || !strings.Contains(err.Error(), "PRODUCT_SPECIFICATION_PATH_CHANGED") {
		t.Fatalf("converged=%v err=%v", converged, err)
	}
}

func TestRunFinalReviewFixesThenRetestsAndReconverges(t *testing.T) {
	ledger := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	convergeCalls, reviewCalls, fixCalls, testCalls := 0, 0, 0, 0
	hooks := FinalReviewHooks{
		Convergence: ConvergenceHooks{
			Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
				return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: ledger, GateVerdicts: map[string]string{"tests": "PASS"}}, nil
			},
			Converge: func(context.Context) (string, error) {
				convergeCalls++
				return "✅ Converged — required gates pass.", nil
			},
			Implement: func(context.Context, []string) error { return nil },
			Test: func(context.Context) (map[string]string, error) {
				testCalls++
				return map[string]string{"tests": "PASS"}, nil
			},
		},
		Review: func(context.Context) ([]string, error) {
			reviewCalls++
			if reviewCalls == 1 {
				return []string{"add missing error handling"}, nil
			}
			return nil, nil
		},
		Fix: func(_ context.Context, findings []string) error {
			fixCalls++
			if len(findings) != 1 || findings[0] != "add missing error handling" {
				t.Fatalf("unexpected review findings: %v", findings)
			}
			return nil
		},
	}
	converged, err := RunFinalReview(context.Background(), hooks, DefaultLoopLimits())
	if err != nil || !converged || convergeCalls != 2 || reviewCalls != 2 || fixCalls != 1 || testCalls != 1 {
		t.Fatalf("converged=%v err=%v converge=%d review=%d fix=%d tests=%d", converged, err, convergeCalls, reviewCalls, fixCalls, testCalls)
	}
}

func TestRunFinalReviewRejectsReplacementSpecificationAfterFix(t *testing.T) {
	path := "specs/001-build-spectide/spec.md"
	ledger := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	hooks := FinalReviewHooks{
		Convergence: ConvergenceHooks{
			Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
				return ConvergenceSnapshot{SpecificationPath: path, TaskLedger: ledger, GateVerdicts: map[string]string{"tests": "PASS"}}, nil
			},
			Converge:  func(context.Context) (string, error) { return "✅ Converged — required gates pass.", nil },
			Implement: func(context.Context, []string) error { return nil },
			Test:      func(context.Context) (map[string]string, error) { return map[string]string{"tests": "PASS"}, nil },
		},
		Review: func(context.Context) ([]string, error) { return []string{"fix issue"}, nil },
		Fix: func(context.Context, []string) error {
			path = "specs/002-replacement/spec.md"
			return nil
		},
	}
	if converged, err := RunFinalReview(context.Background(), hooks, DefaultLoopLimits()); converged || err == nil || !strings.Contains(err.Error(), "PRODUCT_SPECIFICATION_PATH_CHANGED") {
		t.Fatalf("converged=%v err=%v", converged, err)
	}
}

func TestRunFinalReviewStopsWhenSameFindingReturns(t *testing.T) {
	ledger := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	convergeCalls, fixCalls := 0, 0
	hooks := FinalReviewHooks{
		Convergence: ConvergenceHooks{
			Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
				return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: ledger, GateVerdicts: map[string]string{"tests": "PASS"}}, nil
			},
			Converge: func(context.Context) (string, error) {
				convergeCalls++
				return "✅ Converged — required gates pass.", nil
			},
			Implement: func(context.Context, []string) error { return nil },
			Test:      func(context.Context) (map[string]string, error) { return map[string]string{"tests": "PASS"}, nil },
		},
		Review: func(context.Context) ([]string, error) { return []string{"same unresolved issue"}, nil },
		Fix: func(context.Context, []string) error {
			fixCalls++
			return nil
		},
	}
	if converged, err := RunFinalReview(context.Background(), hooks, DefaultLoopLimits()); converged || err == nil || !strings.Contains(err.Error(), "FINAL_REVIEW_NO_PROGRESS") {
		t.Fatalf("converged=%v err=%v", converged, err)
	}
	if convergeCalls != 2 || fixCalls != 1 {
		t.Fatalf("converge calls=%d fix calls=%d", convergeCalls, fixCalls)
	}
}

func TestRunConvergenceRejectsNoProgressAndTaskRewrite(t *testing.T) {
	old := "# Tasks\n\n## Phase 1: Setup\n- [ ] T001 Existing task\n"
	t.Run("no progress", func(t *testing.T) {
		hooks := ConvergenceHooks{
			Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
				return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: old, GateVerdicts: map[string]string{"tests": "FAIL"}}, nil
			},
			Converge: func(context.Context) (string, error) {
				return "## Convergence Findings\nOutcome: tasks_appended\n", nil
			},
			Implement: func(context.Context, []string) error { t.Fatal("implemented without progress"); return nil },
			Test:      func(context.Context) (map[string]string, error) { t.Fatal("tested without progress"); return nil, nil },
		}
		if converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits()); converged || err == nil || !strings.Contains(err.Error(), "CONVERGENCE_TASKS_MISSING") {
			t.Fatalf("converged=%v err=%v", converged, err)
		}
	})
	t.Run("rewrite", func(t *testing.T) {
		ledger := old
		hooks := ConvergenceHooks{
			Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
				return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: ledger}, nil
			},
			Converge: func(context.Context) (string, error) {
				ledger = "# Replaced Tasks\n\n## Phase 2: Convergence\n- [ ] T002 New task\n"
				return "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T002 New task per FR-029\n", nil
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
			return ConvergenceSnapshot{SpecificationPath: "specs/001-build-spectide/spec.md", TaskLedger: ledger}, nil
		},
		Converge: func(context.Context) (string, error) {
			convergeCalls++
			phase++
			task++
			id := task + 1
			ledger += fmt.Sprintf("\n## Phase %d: Convergence\n- [ ] T%03d Appended task per FR-029\n", phase+1, id)
			return fmt.Sprintf("## Convergence Findings\nOutcome: tasks_appended\n- [ ] T%03d Appended task per FR-029\n", id), nil
		},
		Implement: func(context.Context, []string) error { return nil },
		Test:      func(context.Context) (map[string]string, error) { return map[string]string{"tests": "PASS"}, nil },
	}
	converged, err := RunConvergence(context.Background(), hooks, DefaultLoopLimits())
	if converged || err == nil || !strings.Contains(err.Error(), "CONVERGENCE_EXHAUSTED") || convergeCalls != 5 {
		t.Fatalf("converged=%v err=%v convergence calls=%d", converged, err, convergeCalls)
	}
}
