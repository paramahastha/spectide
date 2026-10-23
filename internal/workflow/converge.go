package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/spectide/spectide/internal/speckit"
)

type ConvergenceSnapshot struct {
	SpecificationPath string
	TaskLedger        string
	PlanContent       string
	GateVerdicts      map[string]string
}

type ConvergenceHooks struct {
	Snapshot  func(context.Context) (ConvergenceSnapshot, error)
	Converge  func(context.Context) (string, error)
	Implement func(context.Context, []string) error
	Test      func(context.Context) (map[string]string, error)
	Boundary  func(context.Context, string, []string, int) error
	Resume    *cursorCheckpointState
}

type FinalReviewHooks struct {
	Convergence ConvergenceHooks
	Review      func(context.Context) ([]string, error)
	Fix         func(context.Context, []string) error
	Boundary    func(context.Context, string, []string, int) error
}

// RunConvergence consumes official convergence reports through callbacks while
// enforcing append-only tasks, bounded progress, same-ledger implementation,
// and a required test pass before it asks for another convergence report.
func RunConvergence(ctx context.Context, hooks ConvergenceHooks, limits LoopLimits) (bool, error) {
	budget, err := NewLoopBudget(limits)
	if err != nil {
		return false, err
	}
	return RunConvergenceWithBudget(ctx, hooks, budget, limits)
}

func RunConvergenceWithBudget(ctx context.Context, hooks ConvergenceHooks, budget *LoopBudget, limits LoopLimits) (bool, error) {
	if hooks.Snapshot == nil || hooks.Converge == nil || hooks.Implement == nil || hooks.Test == nil {
		return false, fmt.Errorf("CONVERGENCE_HOOKS_REQUIRED")
	}
	if budget == nil {
		return false, fmt.Errorf("CONVERGENCE_BUDGET_REQUIRED")
	}
	return runConvergenceWithBudget(ctx, hooks, budget, limits)
}

// RunFinalReview returns review findings through the same bounded fix, test,
// and convergence cycle used for appended tasks. All iterations must retain
// the same product specification path and share one per-run convergence budget.
func RunFinalReview(ctx context.Context, hooks FinalReviewHooks, limits LoopLimits) (bool, error) {
	budget, err := NewLoopBudget(limits)
	if err != nil {
		return false, err
	}
	return RunFinalReviewWithBudget(ctx, hooks, budget, limits)
}

func RunFinalReviewWithBudget(ctx context.Context, hooks FinalReviewHooks, budget *LoopBudget, limits LoopLimits) (bool, error) {
	if hooks.Review == nil || hooks.Fix == nil || hooks.Convergence.Snapshot == nil || hooks.Convergence.Converge == nil || hooks.Convergence.Implement == nil || hooks.Convergence.Test == nil {
		return false, fmt.Errorf("FINAL_REVIEW_HOOKS_REQUIRED")
	}
	if budget == nil {
		return false, fmt.Errorf("FINAL_REVIEW_BUDGET_REQUIRED")
	}
	featurePath := ""
	previousReviewFindings := ""
	for {
		snapshot, err := hooks.Convergence.Snapshot(ctx)
		if err != nil {
			return false, err
		}
		if snapshot.SpecificationPath == "" {
			return false, fmt.Errorf("PRODUCT_SPECIFICATION_PATH_REQUIRED")
		}
		if featurePath == "" {
			featurePath = snapshot.SpecificationPath
		} else if snapshot.SpecificationPath != featurePath {
			return false, fmt.Errorf("PRODUCT_SPECIFICATION_PATH_CHANGED")
		}
		converged, err := runConvergenceWithBudget(ctx, hooks.Convergence, budget, limits)
		if err != nil || !converged {
			return false, err
		}
		findings, err := hooks.Review(ctx)
		if err != nil {
			return false, err
		}
		if len(findings) == 0 {
			if hooks.Boundary != nil {
				if err = hooks.Boundary(ctx, "complete_final_review", nil, budget.Counts().ConvergenceRounds); err != nil {
					return false, err
				}
			}
			return true, nil
		}
		findingsHash := hashLedger(strings.Join(findings, "\n"))
		if findingsHash == previousReviewFindings {
			return false, fmt.Errorf("FINAL_REVIEW_NO_PROGRESS")
		}
		previousReviewFindings = findingsHash
		if err = budget.ConvergenceRound(hashLedger("review:"+strings.Join(findings, "\n")), hashLedger("review-fix:"+strings.Join(findings, "\n")), snapshot.GateVerdicts, snapshot.GateVerdicts); err != nil {
			return false, err
		}
		if hooks.Boundary != nil {
			if err = hooks.Boundary(ctx, "fix_final_review_findings", findings, budget.Counts().ConvergenceRounds); err != nil {
				return false, err
			}
		}
		if err = hooks.Fix(ctx, findings); err != nil {
			return false, err
		}
		if hooks.Boundary != nil {
			if err = hooks.Boundary(ctx, "test_after_final_review_fix", findings, 0); err != nil {
				return false, err
			}
		}
		gates, err := hooks.Convergence.Test(ctx)
		if err != nil {
			return false, err
		}
		if len(gates) == 0 {
			return false, fmt.Errorf("REQUIRED_TEST_EVIDENCE_MISSING")
		}
		for name, verdict := range gates {
			if verdict != "PASS" {
				return false, fmt.Errorf("FINAL_REVIEW_FIX_TEST_FAILED: %s=%s", name, verdict)
			}
		}
	}
}

func runConvergenceWithBudget(ctx context.Context, hooks ConvergenceHooks, budget *LoopBudget, limits LoopLimits) (bool, error) {
	if hooks.Resume != nil && hooks.Resume.PendingPhase == "convergence" {
		cursor := hooks.Resume
		switch cursor.PendingOperation {
		case "implement_appended_tasks":
			if len(cursor.PendingTasks) == 0 || cursor.TaskPosition < 1 || cursor.TaskPosition != budget.Counts().ConvergenceRounds {
				return false, fmt.Errorf("WORKSPACE_DRIFT: convergence task cursor is invalid")
			}
			if err := hooks.Implement(ctx, append([]string(nil), cursor.PendingTasks...)); err != nil {
				return false, err
			}
			if hooks.Boundary != nil {
				if err := hooks.Boundary(ctx, "run_tests_after_task_implementation", cursor.PendingTasks, cursor.TaskPosition); err != nil {
					return false, err
				}
			}
			fallthrough
		case "continue_convergence", "run_tests_after_task_implementation":
			gates, err := hooks.Test(ctx)
			if err != nil {
				return false, err
			}
			if err = requirePassingGates(gates, "REQUIRED_TEST"); err != nil {
				return false, err
			}
		case "complete_convergence":
			return true, nil
		}
	}
	for {
		if budget.Counts().ConvergenceRounds >= limits.ConvergenceRounds {
			return false, fmt.Errorf("CONVERGENCE_EXHAUSTED")
		}
		before, err := hooks.Snapshot(ctx)
		if err != nil {
			return false, err
		}
		if before.SpecificationPath == "" {
			return false, fmt.Errorf("PRODUCT_SPECIFICATION_PATH_REQUIRED")
		}
		output, err := hooks.Converge(ctx)
		if err != nil {
			return false, err
		}
		result, err := speckit.ParseConvergenceOutputWithPlan(output, before.PlanContent)
		if err != nil {
			return false, err
		}
		after, err := hooks.Snapshot(ctx)
		if err != nil {
			return false, err
		}
		if after.SpecificationPath != before.SpecificationPath {
			return false, fmt.Errorf("PRODUCT_SPECIFICATION_PATH_CHANGED")
		}
		if result.Converged {
			if before.TaskLedger != after.TaskLedger {
				return false, fmt.Errorf("CONVERGENCE_MODIFIED_TASKS_WHILE_ANALYZING")
			}
			if len(before.GateVerdicts) == 0 {
				return false, fmt.Errorf("REQUIRED_GATE_EVIDENCE_MISSING")
			}
			for name, verdict := range before.GateVerdicts {
				if verdict != "PASS" {
					return false, fmt.Errorf("CONVERGENCE_REQUIRED_GATE_FAILED: %s=%s", name, verdict)
				}
			}
			if hooks.Boundary != nil {
				if err = hooks.Boundary(ctx, "complete_convergence", nil, budget.Counts().ConvergenceRounds); err != nil {
					return false, err
				}
			}
			return true, nil
		}
		ledgerChanged := before.TaskLedger != after.TaskLedger
		if len(result.Appended) > 0 && !ledgerChanged {
			return false, fmt.Errorf("CONVERGENCE_TASKS_NOT_PERSISTED")
		}
		var persistedTasks []string
		if ledgerChanged {
			persistedTasks, err = speckit.AppendedConvergenceTasksWithPlan(before.TaskLedger, after.TaskLedger, before.PlanContent)
			if err != nil {
				return false, err
			}
			if !reflect.DeepEqual(result.Appended, persistedTasks) {
				return false, fmt.Errorf("CONVERGENCE_REPORT_TASKS_MISMATCH")
			}
		}
		if !ledgerChanged && !requiredGateImproved(before.GateVerdicts, after.GateVerdicts) {
			return false, fmt.Errorf("CONVERGENCE_NO_PROGRESS")
		}
		beforeHash := hashLedger(before.TaskLedger)
		afterHash := hashLedger(after.TaskLedger)
		if err = budget.ConvergenceRound(beforeHash, afterHash, before.GateVerdicts, after.GateVerdicts); err != nil {
			return false, err
		}
		if hooks.Boundary != nil {
			operation := "continue_convergence"
			if len(persistedTasks) > 0 {
				operation = "implement_appended_tasks"
			}
			if err = hooks.Boundary(ctx, operation, persistedTasks, budget.Counts().ConvergenceRounds); err != nil {
				return false, err
			}
		}
		if len(persistedTasks) > 0 {
			if err = hooks.Implement(ctx, persistedTasks); err != nil {
				return false, err
			}
			if hooks.Boundary != nil {
				if err = hooks.Boundary(ctx, "run_tests_after_task_implementation", persistedTasks, budget.Counts().ConvergenceRounds); err != nil {
					return false, err
				}
			}
		}
		gates, err := hooks.Test(ctx)
		if err != nil {
			return false, err
		}
		if err = requirePassingGates(gates, "REQUIRED_TEST"); err != nil {
			return false, err
		}
	}
}

func requirePassingGates(gates map[string]string, prefix string) error {
	if len(gates) == 0 {
		return fmt.Errorf("%s_EVIDENCE_MISSING", prefix)
	}
	for name, verdict := range gates {
		if verdict != "PASS" {
			return fmt.Errorf("%s_FAILED: %s=%s", prefix, name, verdict)
		}
	}
	return nil
}

func requiredGateImproved(before, after map[string]string) bool {
	for name, old := range before {
		if old != "PASS" && after[name] == "PASS" {
			return true
		}
	}
	return false
}

func hashLedger(ledger string) string {
	digest := sha256.Sum256([]byte(ledger))
	return hex.EncodeToString(digest[:])
}
