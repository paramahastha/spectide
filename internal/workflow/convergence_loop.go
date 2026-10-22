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

func (e *Engine) liveConvergenceHooks(run model.Run, phase string, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, gateVerdicts map[string]string, budget *LoopBudget) ConvergenceHooks {
	implementRoute := decisions["implement"].Route
	convergeRoute := decisions["converge"].Route
	hooks := ConvergenceHooks{
		Snapshot: func(context.Context) (ConvergenceSnapshot, error) {
			content, err := os.ReadFile(filepath.Join(featureDir, "tasks.md"))
			if err != nil {
				return ConvergenceSnapshot{}, err
			}
			path, err := filepath.Abs(filepath.Join(featureDir, "spec.md"))
			if err != nil {
				return ConvergenceSnapshot{}, err
			}
			return ConvergenceSnapshot{SpecificationPath: filepath.Clean(path), TaskLedger: string(content), GateVerdicts: cloneVerdicts(gateVerdicts)}, nil
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
			result, err := speckit.ParseConvergenceOutput(output)
			if err != nil {
				return "", err
			}
			return output, e.store.Event(ctx, run.ID, "converge", "convergence.report", "info", map[string]any{"converged": result.Converged, "appended_tasks": result.Appended, "execution": "LIVE"})
		},
		Implement: func(ctx context.Context, tasks []string) error {
			prompt := "Implement only the newly appended convergence tasks below. Preserve the existing product specification and do not edit the task ledger.\n"
			for _, task := range tasks {
				prompt += "- " + security.Redact(task) + "\n"
			}
			_, cancelled, err := e.runLiveStageWithControlPhase(ctx, run, "implement", phase+"_implementation", implementRoute, prompt, installation, featureDir)
			if cancelled {
				return context.Canceled
			}
			return err
		},
		Test: func(ctx context.Context) (map[string]string, error) {
			verdicts, err := e.runTestRepairLoop(ctx, run, phase, decisions, installation, featureDir, budget)
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
	return hooks
}

func (e *Engine) runLiveConvergenceLoop(ctx context.Context, run model.Run, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, gateVerdicts map[string]string, budget *LoopBudget) error {
	if decisions["converge"].Route.Provider == "fake" || decisions["implement"].Route.Provider == "fake" {
		return fmt.Errorf("CONVERGENCE_ADAPTER_UNAVAILABLE")
	}
	converged, err := RunConvergenceWithBudget(ctx, e.liveConvergenceHooks(run, "convergence", decisions, installation, featureDir, gateVerdicts, budget), budget, DefaultLoopLimits())
	if err != nil {
		return err
	}
	if !converged {
		return fmt.Errorf("CONVERGENCE_NOT_CONFIRMED")
	}
	return nil
}

func (e *Engine) runLiveFinalReviewLoop(ctx context.Context, run model.Run, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, gateVerdicts map[string]string, budget *LoopBudget) error {
	if decisions["final_review"].Route.Provider == "fake" || decisions["implement"].Route.Provider == "fake" || decisions["converge"].Route.Provider == "fake" {
		return fmt.Errorf("FINAL_REVIEW_ADAPTER_UNAVAILABLE")
	}
	convergence := e.liveConvergenceHooks(run, "final_review", decisions, installation, featureDir, gateVerdicts, budget)
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
			_, cancelled, err := e.runLiveStageWithControlPhase(ctx, run, "implement", "final_review_fix", decisions["implement"].Route, prompt.String(), installation, featureDir)
			if cancelled {
				return context.Canceled
			}
			return err
		},
	}
	converged, err := RunFinalReviewWithBudget(ctx, hooks, budget, DefaultLoopLimits())
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
