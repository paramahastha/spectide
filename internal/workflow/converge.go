package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/spectide/spectide/internal/speckit"
)

type ConvergenceSnapshot struct {
	TaskLedger   string
	GateVerdicts map[string]string
}

type ConvergenceHooks struct {
	Snapshot  func(context.Context) (ConvergenceSnapshot, error)
	Converge  func(context.Context) (string, error)
	Implement func(context.Context, []string) error
	Test      func(context.Context) (map[string]string, error)
}

// RunConvergence consumes official convergence reports through callbacks while
// enforcing append-only tasks, bounded progress, same-ledger implementation,
// and a required test pass before it asks for another convergence report.
func RunConvergence(ctx context.Context, hooks ConvergenceHooks, limits LoopLimits) (bool, error) {
	if hooks.Snapshot == nil || hooks.Converge == nil || hooks.Implement == nil || hooks.Test == nil {
		return false, fmt.Errorf("CONVERGENCE_HOOKS_REQUIRED")
	}
	budget, err := NewLoopBudget(limits)
	if err != nil {
		return false, err
	}
	for {
		if budget.Counts().ConvergenceRounds >= limits.ConvergenceRounds {
			return false, fmt.Errorf("CONVERGENCE_EXHAUSTED")
		}
		before, err := hooks.Snapshot(ctx)
		if err != nil {
			return false, err
		}
		output, err := hooks.Converge(ctx)
		if err != nil {
			return false, err
		}
		result, err := speckit.ParseConvergenceOutput(output)
		if err != nil {
			return false, err
		}
		after, err := hooks.Snapshot(ctx)
		if err != nil {
			return false, err
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
			return true, nil
		}
		ledgerChanged := before.TaskLedger != after.TaskLedger
		if ledgerChanged {
			if err = speckit.ValidateAppendOnly(before.TaskLedger, after.TaskLedger); err != nil {
				return false, err
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
		if len(result.Appended) > 0 {
			if !ledgerChanged {
				return false, fmt.Errorf("CONVERGENCE_TASKS_NOT_PERSISTED")
			}
			if err = hooks.Implement(ctx, result.Appended); err != nil {
				return false, err
			}
		}
		gates, err := hooks.Test(ctx)
		if err != nil {
			return false, err
		}
		if len(gates) == 0 {
			return false, fmt.Errorf("REQUIRED_TEST_EVIDENCE_MISSING")
		}
		for name, verdict := range gates {
			if verdict != "PASS" {
				return false, fmt.Errorf("REQUIRED_TEST_FAILED: %s=%s", name, verdict)
			}
		}
	}
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
