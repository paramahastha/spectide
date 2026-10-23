package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/speckit"
)

func (e *Engine) liveConvergenceHooks(run model.Run, phase string, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, gateVerdicts map[string]string, budget *LoopBudget, restored ...*cursorCheckpointState) ConvergenceHooks {
	implementRoute := decisions["implement"].Route
	convergeRoute := decisions["converge"].Route
	var testResume *cursorCheckpointState
	if len(restored) > 0 {
		testResume = restored[0]
	}
	hooks := ConvergenceHooks{
		Phase: phase,
		Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
			content, err := os.ReadFile(filepath.Join(featureDir, "tasks.md"))
			if err != nil {
				return ConvergenceSnapshot{}, err
			}
			plan, err := os.ReadFile(filepath.Join(featureDir, "plan.md"))
			if err != nil {
				return ConvergenceSnapshot{}, err
			}
			path, err := filepath.Abs(filepath.Join(featureDir, "spec.md"))
			if err != nil {
				return ConvergenceSnapshot{}, err
			}
			return ConvergenceSnapshot{SpecificationPath: filepath.Clean(path), TaskLedger: string(content), PlanContent: string(plan), GateVerdicts: cloneVerdicts(gateVerdicts)}, nil
		},
		Converge: func(ctx context.Context) (string, error) {
			stagePhase := "convergence"
			if phase == "final_review" {
				stagePhase = "final_review_convergence"
			}
			evidence, cancelled, err := e.runLiveStageWithControlPhase(ctx, run, "converge", stagePhase, convergeRoute, run.Prompt, installation, featureDir)
			if err != nil {
				if cancelled {
					return "", context.Canceled
				}
				return "", err
			}
			output, ok := evidence["_stage_output"].(string)
			if !ok {
				return "", fmt.Errorf("CONVERGENCE_OUTPUT_MISSING")
			}
			plan, err := os.ReadFile(filepath.Join(featureDir, "plan.md"))
			if err != nil {
				return "", err
			}
			result, err := speckit.ParseConvergenceOutputWithPlan(output, string(plan))
			if err != nil {
				return "", err
			}
			if err := e.store.Event(ctx, run.ID, "converge", "convergence.report", "info", map[string]any{"converged": result.Converged, "appended_tasks": result.Appended, "execution": "LIVE"}); err != nil {
				return "", err
			}
			return output, nil
		},
		Implement: func(ctx context.Context, tasks []string) error {
			prompt := "Implement only the newly appended convergence tasks below. Preserve the existing product specification and do not edit the task ledger.\n"
			for _, task := range tasks {
				prompt += "- " + security.Redact(task) + "\n"
			}
			continuation := &checkpointContinuation{Phase: phase, Operation: "implement_appended_tasks", Tasks: append([]string(nil), tasks...), Position: budget.Counts().ConvergenceRounds}
			_, cancelled, err := e.runLiveStageWithControlOptions(ctx, run, "implement", phase+"_implementation", implementRoute, prompt, installation, featureDir, liveStageOptions{LoopCounts: budget.Counts(), HasLoopCounts: true, ApprovalContinuation: continuation})
			if cancelled {
				return context.Canceled
			}
			if err != nil {
				return err
			}
			return nil
		},
		Boundary: func(ctx context.Context, operation string, tasks []string, position int) error {
			workflowStage := "converge"
			if phase == "final_review" {
				workflowStage = "final_review"
			}
			return e.loopControlBoundaryAt(ctx, run, workflowStage, featureDir, budget, checkpointCursor{Phase: phase, Operation: operation, Tasks: tasks, Position: position})
		},
		Test: func(ctx context.Context) (map[string]string, error) {
			currentResume := testResume
			testResume = nil
			verdicts, err := e.runTestRepairLoop(ctx, run, phase, decisions, installation, featureDir, budget, currentResume)
			if persistErr := persistTestVerdicts(ctx, e.store, run.ID, verdicts); persistErr != nil {
				return verdicts, persistErr
			}
			if err != nil {
				return verdicts, err
			}
			if allPassedVerdicts(verdicts) {
				gateVerdicts["required_tests"] = "PASS"
			} else {
				gateVerdicts["required_tests"] = "FAIL"
			}
			return verdicts, nil
		},
	}
	if len(restored) > 0 {
		hooks.Resume = restored[0]
	}
	return hooks
}

func (e *Engine) runLiveConvergenceLoop(ctx context.Context, run model.Run, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, gateVerdicts map[string]string, budget *LoopBudget, restored ...*cursorCheckpointState) error {
	if decisions["converge"].Route.Provider == "fake" || decisions["implement"].Route.Provider == "fake" {
		return fmt.Errorf("CONVERGENCE_ADAPTER_UNAVAILABLE")
	}
	converged, err := RunConvergenceWithBudget(ctx, e.liveConvergenceHooks(run, "convergence", decisions, installation, featureDir, gateVerdicts, budget, restored...), budget, budget.Limits())
	if err != nil {
		return err
	}
	if !converged {
		return fmt.Errorf("CONVERGENCE_NOT_CONFIRMED")
	}
	return nil
}

func (e *Engine) runLiveFinalReviewLoop(ctx context.Context, run model.Run, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, gateVerdicts map[string]string, budget *LoopBudget, restored ...*cursorCheckpointState) error {
	if decisions["final_review"].Route.Provider == "fake" || decisions["implement"].Route.Provider == "fake" || decisions["converge"].Route.Provider == "fake" {
		return fmt.Errorf("FINAL_REVIEW_ADAPTER_UNAVAILABLE")
	}
	convergence := e.liveConvergenceHooks(run, "final_review", decisions, installation, featureDir, gateVerdicts, budget, restored...)
	hooks := FinalReviewHooks{
		Convergence: convergence,
		Review: func(ctx context.Context) ([]string, error) {
			prompt := run.Prompt + "\n\nPerform a read-only final review of implementation against the canonical specification, plan, task ledger, and required quality gates. Return findings in the Spec-Kit Specification Analysis Report format. Do not edit files."
			evidence, cancelled, err := e.runLiveStageWithControlPhase(ctx, run, "analyze", "final_review", decisions["final_review"].Route, prompt, installation, featureDir)
			if cancelled {
				return nil, context.Canceled
			}
			if err != nil {
				return nil, err
			}
			output, ok := evidence["_stage_output"].(string)
			if !ok {
				return nil, fmt.Errorf("FINAL_REVIEW_OUTPUT_MISSING")
			}
			analysis, err := speckit.ParseAnalysisOutput(output)
			if err != nil {
				return nil, err
			}
			findings := make([]string, 0, len(analysis.Findings))
			for _, finding := range analysis.Findings {
				findings = append(findings, security.Redact(fmt.Sprintf("%s %s %s: %s; resolution: %s", finding.ID, finding.Severity, finding.Location, finding.Summary, finding.Resolution)))
			}
			return findings, nil
		},
		Fix: func(ctx context.Context, findings []string) error {
			var prompt strings.Builder
			prompt.WriteString("Fix only these final-review findings. Preserve the canonical product specification path and do not replace or rewrite the task ledger.\n")
			for _, finding := range findings {
				prompt.WriteString("- ")
				prompt.WriteString(security.Redact(finding))
				prompt.WriteByte('\n')
			}
			continuation := &checkpointContinuation{Phase: "final_review", Operation: "fix_final_review_findings", Tasks: append([]string(nil), findings...), Position: budget.Counts().ConvergenceRounds}
			_, cancelled, err := e.runLiveStageWithControlOptions(ctx, run, "implement", "final_review_fix", decisions["implement"].Route, prompt.String(), installation, featureDir, liveStageOptions{LoopCounts: budget.Counts(), HasLoopCounts: true, ApprovalContinuation: continuation})
			if cancelled {
				return context.Canceled
			}
			if err != nil {
				return err
			}
			return nil
		},
		Boundary: func(ctx context.Context, operation string, tasks []string, position int) error {
			return e.loopControlBoundaryAt(ctx, run, "final_review", featureDir, budget, checkpointCursor{Phase: "final_review", Operation: operation, Tasks: tasks, Position: position})
		},
	}
	if len(restored) > 0 {
		hooks.Resume = restored[0]
	}
	converged, err := RunFinalReviewWithBudget(ctx, hooks, budget, budget.Limits())
	if err != nil {
		return err
	}
	if !converged {
		return fmt.Errorf("FINAL_REVIEW_NOT_CONFIRMED")
	}
	return nil
}

func cloneVerdicts(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for name, verdict := range source {
		clone[name] = verdict
	}
	return clone
}

func allPassedVerdicts(verdicts map[string]string) bool {
	if len(verdicts) == 0 {
		return false
	}
	for _, verdict := range verdicts {
		if verdict != "PASS" {
			return false
		}
	}
	return true
}
