package workflow

import (
	"context"
	"crypto/sha256"
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
			return fingerprintArtifacts(featureDir)
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
		Remediate: func(ctx context.Context, finding speckit.Finding) error {
			before, err := artifactDigests(featureDir)
			if err != nil {
				return err
			}
			prompt := strings.Join([]string{
				"Remediate exactly one blocking Spec-Kit analyze finding in its owning artifact. Do not edit any other artifact or unrelated project file.",
				"Owning artifact: " + finding.Owner,
				"Finding ID: " + finding.ID,
				"Category: " + security.Redact(finding.Category),
				"Severity: " + finding.Severity,
				"Location: " + security.Redact(finding.Location),
				"Finding: " + security.Redact(finding.Summary),
				"Resolution: " + security.Redact(finding.Resolution),
			}, "\n")
			if _, cancelled, runErr := e.runLiveStageWithControl(ctx, run, "implement", implementRoute, prompt, installation, featureDir); runErr != nil {
				if cancelled {
					return context.Canceled
				}
				return runErr
			}
			after, err := artifactDigests(featureDir)
			if err != nil {
				return err
			}
			if err := validateOwnerArtifactChange(before, after, finding.Owner); err != nil {
				return err
			}
			return e.store.Event(ctx, run.ID, "analyze", "analysis.remediated", "info", map[string]any{"finding_id": finding.ID, "owner": finding.Owner, "severity": finding.Severity})
		},
	}
	return RunAnalyzeGateWithBudget(ctx, hooks, budget, DefaultLoopLimits())
}

func validateOwnerArtifactChange(before, after map[string]string, owner string) error {
	if owner != "spec.md" && owner != "plan.md" && owner != "tasks.md" {
		return fmt.Errorf("ANALYSIS_OWNER_UNRESOLVED")
	}
	if before[owner] == after[owner] {
		return fmt.Errorf("ANALYZE_REMEDIATION_NO_PROGRESS")
	}
	for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
		if name != owner && before[name] != after[name] {
			return fmt.Errorf("ANALYZE_REMEDIATION_CHANGED_NON_OWNER_ARTIFACT")
		}
	}
	return nil
}

func artifactDigests(featureDir string) (map[string]string, error) {
	state, err := speckit.InspectArtifacts(featureDir)
	if err != nil {
		return nil, err
	}
	digests := make(map[string]string, 3)
	for name, path := range map[string]string{"spec.md": state.SpecPath, "plan.md": state.PlanPath, "tasks.md": state.TasksPath} {
		if !state.Present[strings.TrimSuffix(name, ".md")] {
			digests[name] = "absent"
			continue
		}
		content, readErr := os.ReadFile(filepath.Clean(path))
		if readErr != nil {
			return nil, readErr
		}
		digest := sha256.Sum256(content)
		digests[name] = fmt.Sprintf("%x", digest[:])
	}
	return digests, nil
}
