package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/spectide/spectide/internal/speckit"
)

type AnalyzeHooks struct {
	ArtifactHash func(context.Context) (string, error)
	Analyze      func(context.Context) (string, error)
	ResolveOwner func(speckit.Finding) (string, error)
	Remediate    func(context.Context, speckit.Finding) error
}

// RunAnalyzeGate performs a read-only baseline analysis, then addresses one
// blocking finding per owning-artifact change and read-only re-analysis.
func RunAnalyzeGate(ctx context.Context, hooks AnalyzeHooks, limits LoopLimits) (speckit.Analysis, error) {
	budget, err := NewLoopBudget(limits)
	if err != nil {
		return speckit.Analysis{}, err
	}
	return RunAnalyzeGateWithBudget(ctx, hooks, budget, limits)
}

func RunAnalyzeGateWithBudget(ctx context.Context, hooks AnalyzeHooks, budget *LoopBudget, limits LoopLimits) (speckit.Analysis, error) {
	if hooks.ArtifactHash == nil || hooks.Analyze == nil || hooks.Remediate == nil {
		return speckit.Analysis{}, fmt.Errorf("ANALYZE_HOOKS_REQUIRED")
	}
	if budget == nil {
		return speckit.Analysis{}, fmt.Errorf("ANALYZE_BUDGET_REQUIRED")
	}
	analysis, artifactHash, err := readOnlyAnalysis(ctx, hooks)
	if err != nil {
		return analysis, err
	}
	for {
		blocking := BlockingAnalysisFindings(analysis)
		if len(blocking) == 0 {
			return analysis, nil
		}
		if budget.Counts().AnalyzeRemediations >= limits.AnalyzeRemediations {
			return analysis, fmt.Errorf("ANALYZE_REMEDIATION_EXHAUSTED")
		}
		finding := blocking[0]
		if hooks.ResolveOwner != nil {
			finding.Owner, err = hooks.ResolveOwner(finding)
		} else {
			finding.Owner, err = AnalysisOwner(finding)
		}
		if err != nil {
			return analysis, err
		}
		if err = hooks.Remediate(ctx, finding); err != nil {
			return analysis, err
		}
		remediatedHash, hashErr := hooks.ArtifactHash(ctx)
		if hashErr != nil {
			return analysis, hashErr
		}
		if err = budget.AnalysisRemediation(artifactHash, remediatedHash); err != nil {
			return analysis, err
		}
		analysis, artifactHash, err = readOnlyAnalysis(ctx, hooks)
		if err != nil {
			return analysis, err
		}
	}
}

// AnalysisOwner maps blocking findings to the artifact that owns the affected
// requirement. Unknown findings fail closed rather than editing an arbitrary
// project artifact.
func AnalysisOwner(f speckit.Finding) (string, error) {
	text := strings.ToLower(strings.Join([]string{f.Category, f.Location, f.Summary, f.Resolution}, " "))
	switch {
	case strings.Contains(text, "tasks.md"), strings.Contains(text, "task coverage"), strings.Contains(text, "task ordering"), strings.Contains(text, "dependency order"):
		return "tasks.md", nil
	case strings.Contains(text, "plan.md"), strings.Contains(text, "architecture"), strings.Contains(text, "data model"), strings.Contains(text, "api contract"), strings.Contains(text, "design contract"):
		return "plan.md", nil
	case strings.Contains(text, "spec.md"), strings.Contains(text, "scope"), strings.Contains(text, "acceptance"), strings.Contains(text, "security boundary"), strings.Contains(text, "user requirement"):
		return "spec.md", nil
	default:
		return "", fmt.Errorf("ANALYSIS_OWNER_UNRESOLVED: %s", f.ID)
	}
}

func readOnlyAnalysis(ctx context.Context, hooks AnalyzeHooks) (speckit.Analysis, string, error) {
	before, err := hooks.ArtifactHash(ctx)
	if err != nil {
		return speckit.Analysis{}, "", err
	}
	if before == "" {
		return speckit.Analysis{}, "", fmt.Errorf("ANALYZE_ARTIFACT_HASH_REQUIRED")
	}
	output, err := hooks.Analyze(ctx)
	if err != nil {
		return speckit.Analysis{}, "", err
	}
	after, err := hooks.ArtifactHash(ctx)
	if err != nil {
		return speckit.Analysis{}, "", err
	}
	if before != after {
		return speckit.Analysis{}, "", fmt.Errorf("ANALYZE_MUTATED_ARTIFACTS")
	}
	analysis, err := speckit.ParseAnalysisOutput(output)
	return analysis, after, err
}

// BlockingAnalysisFindings applies FR-008 severity policy. Critical/high
// findings always block. Medium findings block only when the finding
// explicitly identifies a security boundary, required acceptance, or
// prerequisite as unsatisfied.
func BlockingAnalysisFindings(analysis speckit.Analysis) []speckit.Finding {
	var blocking []speckit.Finding
	for _, finding := range analysis.Findings {
		switch finding.Severity {
		case "CRITICAL", "HIGH":
			blocking = append(blocking, finding)
		case "MEDIUM":
			if mediumFindingBlocks(finding) {
				blocking = append(blocking, finding)
			}
		}
	}
	return blocking
}

func mediumFindingBlocks(f speckit.Finding) bool {
	category := strings.ToLower(f.Category)
	text := strings.ToLower(strings.Join([]string{f.Location, f.Summary, f.Resolution}, " "))
	containsAny := func(terms ...string) bool {
		for _, term := range terms {
			if strings.Contains(text, term) {
				return true
			}
		}
		return false
	}
	if strings.Contains(category, "security") && containsAny("boundary", "unsafe", "violated", "unsatisfied") {
		return true
	}
	if (strings.Contains(category, "acceptance") || strings.Contains(category, "requirement coverage")) && containsAny("unsatisfied", "uncovered", "missing", "not met", "fails") {
		return true
	}
	if (strings.Contains(category, "prerequisite") || strings.Contains(category, "dependency")) && containsAny("unsatisfied", "missing", "blocked", "not met") {
		return true
	}
	return false
}
