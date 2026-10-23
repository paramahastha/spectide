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

func (e *Engine) runTestRepairLoop(ctx context.Context, run model.Run, phase string, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, budget *LoopBudget, restored ...*cursorCheckpointState) (map[string]string, error) {
	latest := map[string]string{}
	var resumeCursor *cursorCheckpointState
	if len(restored) > 0 && restored[0] != nil && restored[0].PendingPhase == phase && restored[0].PendingOperation == "continue_registered_checks" {
		resumeCursor = restored[0]
	}
	hooks := TestRepairHooks{
		Test: func(ctx context.Context) (map[string]string, error) {
			workflowStage := phase
			switch phase {
			case "convergence":
				workflowStage = "converge"
			case "final_review":
				workflowStage = "final_review"
			}
			currentResume := resumeCursor
			resumeCursor = nil
			completedIDs := []string{}
			if currentResume != nil {
				completedIDs = append(completedIDs, currentResume.CompletedQualityExecutionIDs...)
			}
			startCursor := checkpointCursor{Phase: phase, Operation: "start_registered_checks"}
			if currentResume != nil {
				startCursor = checkpointCursor{Phase: phase, Operation: currentResume.PendingOperation, Check: currentResume.PendingCheck, Position: currentResume.TaskPosition, CompletedQualityExecutionIDs: append([]string(nil), currentResume.CompletedQualityExecutionIDs...)}
			}
			if err := e.loopControlBoundaryAt(ctx, run, workflowStage, featureDir, budget, startCursor); err != nil {
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
			registry, err := e.qualityRegistryForRun(ctx, run, effectiveConfiguration)
			if err != nil {
				return nil, err
			}
			completed := map[string]string{}
			if currentResume != nil {
				completed, err = restoreCompletedQualityResults(ctx, e.store, run.ID, phase, registry.Checks(), *currentResume)
				if err != nil {
					return nil, err
				}
			}
			completedKeys := make(map[string]struct{}, len(completed))
			for key := range completed {
				completedKeys[key] = struct{}{}
			}
			checksToRun := registry.Except(completedKeys)
			var inheritedFiles []*os.File
			e.mu.Lock()
			if processLock := e.processLocks[run.ID]; processLock != nil && processLock.File() != nil {
				inheritedFiles = append(inheritedFiles, processLock.File())
			}
			e.mu.Unlock()
			evidence, runErr := checksToRun.RunControlled(ctx, run.Project, func(ctx context.Context) (string, error) {
				return e.store.RunControl(ctx, run.ID)
			}, func(ctx context.Context, item quality.Evidence) error {
				startedAt := item.StartedAt
				if startedAt.IsZero() {
					startedAt = time.Now().UTC().Add(-item.Duration)
				}
				finishedAt := startedAt.Add(item.Duration)
				persisted := model.QualityExecution{RunID: run.ID, Phase: phase, ProcessID: os.Getpid(), StartedAt: startedAt, FinishedAt: finishedAt}
				persisted.Results = []model.QualityResult{{Key: item.Key, Verdict: string(item.Verdict), Command: append([]string(nil), item.Command...), ProcessID: item.ProcessID, ExitCode: item.ExitCode, Duration: item.Duration, Summary: item.Summary, StartedAt: startedAt}}
				persisted, err = e.store.SaveQualityExecution(ctx, persisted)
				if err != nil {
					return err
				}
				completedIDs = append(completedIDs, persisted.ID)
				if err := e.store.Gate(ctx, run.ID, "live_test_"+item.Key, string(item.Verdict), security.Redact(item.Summary)); err != nil {
					return err
				}
				return e.loopControlBoundaryAt(ctx, run, workflowStage, featureDir, budget, checkpointCursor{Phase: phase, Operation: "continue_registered_checks", Check: item.Key, Position: len(completedIDs), CompletedQualityExecutionIDs: append([]string(nil), completedIDs...)})
			}, inheritedFiles...)
			verdicts := make(map[string]string, len(completed)+len(evidence))
			for key, verdict := range completed {
				verdicts[key] = verdict
			}
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
			// Approval pauses before this write. Resume by rerunning the checks and
			// repair, but roll back the pre-invocation budget charge in the durable
			// cursor so the resumed repair consumes that same single slot.
			continuation := &checkpointContinuation{Phase: phase, Operation: "restart_registered_checks"}
			counts := testRepairRestartCounts(budget.Counts())
			_, _, err := e.runLiveStageWithControlOptions(ctx, run, "implement", phase+"_repair", decision.Route, prompt.String(), installation, featureDir, liveStageOptions{LoopCounts: counts, HasLoopCounts: true, ApprovalContinuation: continuation})
			if err != nil {
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

func testRepairRestartCounts(counts LoopLimits) LoopLimits {
	if counts.TestRepairs > 0 {
		counts.TestRepairs--
	}
	return counts
}

func restoreCompletedQualityResults(ctx context.Context, state interface {
	Report(context.Context, string) (model.Report, error)
}, runID, phase string, checks []quality.Check, cursor cursorCheckpointState) (map[string]string, error) {
	if cursor.TaskPosition < 1 || cursor.TaskPosition != len(cursor.CompletedQualityExecutionIDs) || cursor.TaskPosition > len(checks) {
		return nil, fmt.Errorf("WORKSPACE_DRIFT: checkpoint quality cursor position is invalid")
	}
	seen := make(map[string]struct{}, len(cursor.CompletedQualityExecutionIDs))
	for _, id := range cursor.CompletedQualityExecutionIDs {
		if id == "" {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: checkpoint quality execution identity is missing")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: checkpoint repeats a quality execution identity")
		}
		seen[id] = struct{}{}
	}
	report, err := state.Report(ctx, runID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]model.QualityExecution, len(report.QualityExecutions))
	for _, execution := range report.QualityExecutions {
		byID[execution.ID] = execution
	}
	results := make(map[string]string, len(seen))
	for index, id := range cursor.CompletedQualityExecutionIDs {
		execution, exists := byID[id]
		if !exists || execution.Phase != phase || len(execution.Results) != 1 || index >= len(checks) {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: checkpoint quality evidence is unavailable")
		}
		result := execution.Results[0]
		if result.Key != checks[index].Key {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: checkpoint quality check order changed")
		}
		if result.Verdict != "PASS" && result.Verdict != "FAIL" && result.Verdict != "BLOCKED" && result.Verdict != "NOT_RUN" {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: checkpoint quality verdict is invalid")
		}
		results[result.Key] = result.Verdict
	}
	if len(cursor.CompletedQualityExecutionIDs) == 0 || cursor.PendingCheck != checks[len(cursor.CompletedQualityExecutionIDs)-1].Key {
		return nil, fmt.Errorf("WORKSPACE_DRIFT: checkpoint quality cursor check does not match evidence")
	}
	return results, nil
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
