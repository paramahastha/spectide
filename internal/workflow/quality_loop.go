package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/quality"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/speckit"
)

func (e *Engine) runTestRepairLoop(ctx context.Context, run model.Run, decisions map[string]config.Decision, installation speckit.Installation, featureDir string) (map[string]string, error) {
	latest := map[string]string{}
	hooks := TestRepairHooks{
		Test: func(ctx context.Context) (map[string]string, error) {
			registry := quality.DefaultRegistry(nil)
			evidence := registry.Run(ctx, run.Project)
			verdicts := make(map[string]string, len(evidence))
			for _, item := range evidence {
				verdict := string(item.Verdict)
				verdicts[item.Key] = verdict
				summary := security.Redact(item.Summary)
				if err := e.store.Gate(ctx, run.ID, "live_test_"+item.Key, verdict, summary); err != nil {
					return nil, err
				}
			}
			latest = verdicts
			return verdicts, nil
		},
		Repair: func(ctx context.Context, failed map[string]string) error {
			decision, ok := decisions["implement"]
			if !ok || decision.Route.Provider == "fake" {
				return fmt.Errorf("TEST_REPAIR_ADAPTER_UNAVAILABLE")
			}
			keys := make([]string, 0, len(failed))
			for key := range failed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			var prompt strings.Builder
			prompt.WriteString("Repair only these failed registered checks. Do not change unrelated requirements.\n")
			for _, key := range keys {
				fmt.Fprintf(&prompt, "- %s: %s\n", key, failed[key])
			}
			if _, _, err := e.runLiveStageWithControl(ctx, run, "implement", decision.Route, prompt.String(), installation, featureDir); err != nil {
				return err
			}
			return nil
		},
	}
	verdicts, err := RunTestRepairGate(ctx, hooks, DefaultLoopLimits())
	if err != nil {
		return verdicts, err
	}
	latest = verdicts
	return latest, nil
}

func persistTestVerdicts(ctx context.Context, state interface {
	Gate(context.Context, string, string, string, string) error
}, runID string, verdicts map[string]string) {
	allPassed := len(verdicts) > 0
	for key, verdict := range verdicts {
		if verdict != "PASS" {
			allPassed = false
		}
		_ = state.Gate(ctx, runID, "live_test_"+key, verdict, "registered local quality check")
	}
	if allPassed {
		_ = state.Gate(ctx, runID, "required_tests", "PASS", fmt.Sprintf("%d registered local checks passed", len(verdicts)))
	} else if len(verdicts) > 0 {
		_ = state.Gate(ctx, runID, "required_tests", "FAIL", "one or more registered local checks did not pass")
	} else {
		_ = state.Gate(ctx, runID, "required_tests", "BLOCKED", "no registered local check evidence was produced")
	}
}
