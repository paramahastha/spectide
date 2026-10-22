package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type TestRepairHooks struct {
	Test   func(context.Context) (map[string]string, error)
	Repair func(context.Context, map[string]string) error
}

// RunTestRepairGate runs a baseline test, then allows at most the configured
// number of repairs. A repair is useful only when the failing verdict set
// changes; an unchanged failure set blocks immediately.
func RunTestRepairGate(ctx context.Context, hooks TestRepairHooks, limits LoopLimits) (map[string]string, error) {
	budget, err := NewLoopBudget(limits)
	if err != nil {
		return nil, err
	}
	return RunTestRepairGateWithBudget(ctx, hooks, budget, limits)
}

func RunTestRepairGateWithBudget(ctx context.Context, hooks TestRepairHooks, budget *LoopBudget, limits LoopLimits) (map[string]string, error) {
	if hooks.Test == nil || hooks.Repair == nil {
		return nil, fmt.Errorf("TEST_REPAIR_HOOKS_REQUIRED")
	}
	if budget == nil {
		return nil, fmt.Errorf("TEST_REPAIR_BUDGET_REQUIRED")
	}
	verdicts, err := hooks.Test(ctx)
	if err != nil {
		return nil, err
	}
	if err = validateTestVerdicts(verdicts); err != nil {
		return verdicts, err
	}
	if containsUnavailableTestVerdict(verdicts) {
		return verdicts, fmt.Errorf("REQUIRED_TEST_CHECK_BLOCKED")
	}
	for {
		failed := failedTestVerdicts(verdicts)
		if len(failed) == 0 {
			return verdicts, nil
		}
		if err = budget.TestRepair(); err != nil {
			return verdicts, err
		}
		if err = hooks.Repair(ctx, failed); err != nil {
			return verdicts, err
		}
		next, testErr := hooks.Test(ctx)
		if testErr != nil {
			return nil, testErr
		}
		if err = validateTestVerdicts(next); err != nil {
			return next, err
		}
		if containsUnavailableTestVerdict(next) {
			return next, fmt.Errorf("REQUIRED_TEST_CHECK_BLOCKED")
		}
		if verdictFingerprint(verdicts) == verdictFingerprint(next) {
			return next, fmt.Errorf("TEST_REPAIR_NO_PROGRESS")
		}
		verdicts = next
	}
}

func containsUnavailableTestVerdict(verdicts map[string]string) bool {
	for _, verdict := range verdicts {
		if verdict == "BLOCKED" || verdict == "NOT_RUN" {
			return true
		}
	}
	return false
}

func validateTestVerdicts(verdicts map[string]string) error {
	if len(verdicts) == 0 {
		return fmt.Errorf("REQUIRED_TEST_EVIDENCE_MISSING")
	}
	for name, verdict := range verdicts {
		if verdict != "PASS" && verdict != "FAIL" && verdict != "BLOCKED" && verdict != "NOT_RUN" {
			return fmt.Errorf("INVALID_TEST_VERDICT: %s=%s", name, verdict)
		}
	}
	return nil
}

func failedTestVerdicts(verdicts map[string]string) map[string]string {
	failed := make(map[string]string)
	for name, verdict := range verdicts {
		if verdict != "PASS" {
			failed[name] = verdict
		}
	}
	return failed
}

func verdictFingerprint(verdicts map[string]string) string {
	keys := make([]string, 0, len(verdicts))
	for key := range verdicts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var canonical strings.Builder
	for _, key := range keys {
		canonical.WriteString(key)
		canonical.WriteByte('=')
		canonical.WriteString(verdicts[key])
		canonical.WriteByte('\n')
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(digest[:])
}
