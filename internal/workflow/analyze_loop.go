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

func (e *Engine) runLiveAnalyzeLoop(ctx context.Context, run model.Run, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, budget *LoopBudget) (speckit.Analysis, error) {
	analysisRoute := decisions["analyze"].Route
	implementRoute := decisions["implement"].Route
	if analysisRoute.Provider == "fake" {
		return speckit.Analysis{}, fmt.Errorf("ANALYZE_REMEDIATION_ADAPTER_UNAVAILABLE")
	}
	hooks := AnalyzeHooks{
		ArtifactHash: func(context.Context) (string, error) {
			return workspaceFingerprint(installation.ProjectRoot)
		},
		ResolveOwner: func(finding speckit.Finding) (string, error) {
			owner, _, err := resolveAnalysisOwner(installation.ProjectRoot, featureDir, finding)
			return owner, err
		},
		Analyze: func(ctx context.Context) (string, error) {
			evidence, cancelled, err := e.runLiveStageWithControl(ctx, run, "analyze", analysisRoute, run.Prompt, installation, featureDir)
			if err != nil {
				if cancelled {
					return "", context.Canceled
				}
				return "", err
			}
			output, ok := evidence["_stage_output"].(string)
			if !ok {
				return "", fmt.Errorf("ANALYZE_OUTPUT_MISSING")
			}
			return output, nil
		},
		Boundary: func(ctx context.Context, operation string) error {
			return e.loopControlBoundaryAt(ctx, run, "analyze", featureDir, budget, checkpointCursor{Phase: "analyze", Operation: operation})
		},
		Remediate: func(ctx context.Context, finding speckit.Finding) error {
			ownerPath, templateStage, err := resolveAnalysisOwner(installation.ProjectRoot, featureDir, finding)
			if err != nil {
				return err
			}
			before, err := workspaceFiles(installation.ProjectRoot)
			if err != nil {
				return err
			}
			prompt := strings.Join([]string{
				"Remediate exactly one blocking Spec-Kit analyze finding by editing only the owning artifact named below. Do not implement product code, edit any other file, or perform general implementation work.",
				"Owning artifact: " + ownerPath,
				"Finding ID: " + finding.ID,
				"Category: " + security.Redact(finding.Category),
				"Severity: " + finding.Severity,
				"Location: " + security.Redact(finding.Location),
				"Finding: " + security.Redact(finding.Summary),
				"Resolution: " + security.Redact(finding.Resolution),
			}, "\n")
			if _, cancelled, runErr := e.runLiveStageWithControlOptions(ctx, run, "implement", "analyze_remediation", implementRoute, prompt, installation, featureDir, liveStageOptions{templateStage: templateStage, ownerPath: ownerPath, LoopCounts: budget.Counts(), HasLoopCounts: true}); runErr != nil {
				if cancelled {
					return context.Canceled
				}
				return runErr
			}
			after, err := workspaceFiles(installation.ProjectRoot)
			if err != nil {
				return err
			}
			if err := validateWorkspaceOwnerChange(before, after, ownerPath); err != nil {
				return err
			}
			if err := e.store.Event(ctx, run.ID, "analyze", "analysis.remediated", "info", map[string]any{"finding_id": finding.ID, "owner": finding.Owner, "severity": finding.Severity}); err != nil {
				return err
			}
			return nil
		},
	}
	return RunAnalyzeGateWithBudget(ctx, hooks, budget, budget.Limits())
}

func validateWorkspaceOwnerChange(before, after map[string]stageFile, owner string) error {
	if owner == "" {
		return fmt.Errorf("ANALYSIS_OWNER_UNRESOLVED")
	}
	if after[owner].SHA256 == "" || before[owner].SHA256 == after[owner].SHA256 {
		return fmt.Errorf("ANALYZE_REMEDIATION_NO_PROGRESS")
	}
	for path, previous := range before {
		if path != owner && after[path].SHA256 != previous.SHA256 {
			return fmt.Errorf("ANALYZE_REMEDIATION_CHANGED_NON_OWNER_ARTIFACT: %s", path)
		}
	}
	for path := range after {
		if path != owner {
			if _, existed := before[path]; !existed {
				return fmt.Errorf("ANALYZE_REMEDIATION_CHANGED_NON_OWNER_ARTIFACT: %s", path)
			}
		}
	}
	return nil
}

func resolveAnalysisOwner(projectRoot, featureDir string, finding speckit.Finding) (string, string, error) {
	location := filepath.ToSlash(finding.Location)
	if index := strings.Index(location, "contracts/"); index >= 0 {
		candidate := location[index:]
		for i, r := range candidate {
			if r == ':' || r == ')' || r == ']' || r == ',' || r == ' ' || r == '`' {
				candidate = candidate[:i]
				break
			}
		}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(candidate)))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return "", "", fmt.Errorf("ANALYSIS_OWNER_UNRESOLVED: %s", finding.ID)
		}
		planPath := filepath.Join(featureDir, "plan.md")
		plan, err := os.ReadFile(planPath)
		if err != nil {
			return "", "", err
		}
		if !strings.Contains(filepath.ToSlash(string(plan)), clean) {
			return "", "", fmt.Errorf("ANALYSIS_CONTRACT_NOT_REFERENCED_BY_PLAN: %s", clean)
		}
		absolute := filepath.Join(featureDir, filepath.FromSlash(clean))
		info, err := os.Lstat(absolute)
		if err != nil {
			return "", "", fmt.Errorf("ANALYSIS_OWNER_UNRESOLVED: %s", finding.ID)
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", "", err
		}
		canonicalFeature, err := filepath.EvalSymlinks(featureDir)
		if err != nil {
			return "", "", err
		}
		rel, err := filepath.Rel(canonicalFeature, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("ANALYSIS_CONTRACT_PATH_INVALID: %s", clean)
		}
		projectRelative, err := filepath.Rel(projectRoot, absolute)
		if err != nil || projectRelative == ".." || strings.HasPrefix(projectRelative, ".."+string(filepath.Separator)) {
			return "", "", fmt.Errorf("ANALYSIS_CONTRACT_PATH_INVALID: %s", clean)
		}
		return filepath.ToSlash(projectRelative), "plan", nil
	}
	owner, err := AnalysisOwner(finding)
	if err != nil {
		return "", "", err
	}
	absolute := filepath.Join(featureDir, owner)
	projectRelative, err := filepath.Rel(projectRoot, absolute)
	if err != nil || projectRelative == ".." || strings.HasPrefix(projectRelative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("ANALYSIS_OWNER_PATH_INVALID")
	}
	templateStage := map[string]string{"spec.md": "clarify", "plan.md": "plan", "tasks.md": "tasks"}[owner]
	return filepath.ToSlash(projectRelative), templateStage, nil
}
