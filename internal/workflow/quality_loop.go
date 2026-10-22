package workflow

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/quality"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/speckit"
)

func (e *Engine) runTestRepairLoop(ctx context.Context, run model.Run, phase string, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, budget *LoopBudget) (map[string]string, error) {
	latest := map[string]string{}
	hooks := TestRepairHooks{
		Test: func(ctx context.Context) (map[string]string, error) {
			workflowStage := phase
			switch phase {
			case "convergence":
				workflowStage = "converge"
			case "final_review":
				workflowStage = "final_review"
			}
			if err := e.loopControlBoundary(ctx, run, workflowStage, featureDir, budget); err != nil {
				return nil, err
			}
			runConfiguration, err := e.store.RunConfiguration(ctx, run.ID)
			if err != nil {
				return nil, err
			}
			effectiveConfiguration, err := parseEffectiveRunConfiguration(runConfiguration.ConfigJSON)
			if err != nil {
				return nil, err
			}
			registry, err := effectiveConfiguration.qualityRegistry()
			if err != nil {
				return nil, err
			}
			evidence, runErr := registry.RunControlled(ctx, run.Project, func(ctx context.Context) (string, error) {
				return e.store.RunControl(ctx, run.ID)
			}, func(ctx context.Context, item quality.Evidence) error {
				startedAt := item.StartedAt
				if startedAt.IsZero() {
					startedAt = time.Now().UTC().Add(-item.Duration)
				}
				finishedAt := startedAt.Add(item.Duration)
				persisted := model.QualityExecution{RunID: run.ID, Phase: phase, ProcessID: os.Getpid(), StartedAt: startedAt, FinishedAt: finishedAt}
				persisted.Results = []model.QualityResult{{Key: item.Key, Verdict: string(item.Verdict), Command: append([]string(nil), item.Command...), ProcessID: item.ProcessID, ExitCode: item.ExitCode, Duration: item.Duration, Summary: item.Summary, StartedAt: startedAt}}
				if _, err := e.store.SaveQualityExecution(ctx, persisted); err != nil {
					return err
				}
				if err := e.store.Gate(ctx, run.ID, "live_test_"+item.Key, string(item.Verdict), security.Redact(item.Summary)); err != nil {
					return err
				}
				return e.loopControlBoundary(ctx, run, workflowStage, featureDir, budget)
			})
			verdicts := make(map[string]string, len(evidence))
			for _, item := range evidence {
				verdict := string(item.Verdict)
				verdicts[item.Key] = verdict
			}
			latest = verdicts
			return verdicts, runErr
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
			if _, _, err := e.runLiveStageWithControlPhase(ctx, run, "implement", phase+"_repair", decision.Route, prompt.String(), installation, featureDir); err != nil {
				return err
			}
			return nil
		},
	}
	verdicts, err := RunTestRepairGateWithBudget(ctx, hooks, budget, budget.Limits())
	if err != nil {
		return verdicts, err
	}
	latest = verdicts
	return latest, nil
}

func persistTestVerdicts(ctx context.Context, state interface {
	Gate(context.Context, string, string, string, string) error
}, runID string, verdicts map[string]string) error {
	verdict := testGateVerdict(verdicts)
	keys := make([]string, 0, len(verdicts))
	for key := range verdicts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		verdict := verdicts[key]
		if err := state.Gate(ctx, runID, "live_test_"+key, verdict, "registered local quality check"); err != nil {
			return err
		}
	}
	switch verdict {
	case "PASS":
		return state.Gate(ctx, runID, "required_tests", "PASS", fmt.Sprintf("%d registered local checks passed", len(verdicts)))
	case "FAIL":
		return state.Gate(ctx, runID, "required_tests", "FAIL", "one or more registered local checks failed")
	case "BLOCKED":
		return state.Gate(ctx, runID, "required_tests", "BLOCKED", "one or more required local checks could not run")
	default:
		return state.Gate(ctx, runID, "required_tests", "BLOCKED", "required local check evidence was not produced")
	}
}

func testGateVerdict(verdicts map[string]string) string {
	if len(verdicts) == 0 {
		return "BLOCKED"
	}
	blocked, failed := false, false
	for _, verdict := range verdicts {
		if verdict == "FAIL" {
			failed = true
		}
		if verdict == "BLOCKED" || verdict == "NOT_RUN" {
			blocked = true
		}
	}
	if failed {
		return "FAIL"
	}
	if blocked {
		return "BLOCKED"
	}
	return "PASS"
}
