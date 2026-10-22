package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/speckit"
)

const (
	blockingAnalysis = "## Specification Analysis Report\n| C1 | Security | CRITICAL | spec.md:1 | Boundary is unsafe | Fix boundary |\n"
	cleanAnalysis    = "## Specification Analysis Report\n| I1 | Ambiguity | MEDIUM | spec.md:1 | Bound unspecified | Clarify if needed |\n"
)

func TestBlockingAnalysisFindingsFollowFR008SeverityPolicy(t *testing.T) {
	a := speckit.Analysis{Findings: []speckit.Finding{
		{ID: "C1", Severity: "CRITICAL"},
		{ID: "H1", Severity: "HIGH"},
		{ID: "M1", Severity: "MEDIUM", Category: "Requirement Coverage", Summary: "Required acceptance criterion missing"},
		{ID: "M2", Severity: "MEDIUM", Category: "Ambiguity", Summary: "A bound is unspecified"},
		{ID: "L1", Severity: "LOW"},
	}}
	got := BlockingAnalysisFindings(a)
	if len(got) != 3 || got[0].ID != "C1" || got[1].ID != "H1" || got[2].ID != "M1" {
		t.Fatalf("blocking findings=%+v", got)
	}
}

func TestRunAnalyzeGateExcludesBaselineAndRemediatesThenReanalyzes(t *testing.T) {
	artifactHash, analyzeCalls, remediationCalls := "artifact-0", 0, 0
	hooks := AnalyzeHooks{
		ArtifactHash: func(context.Context) (string, error) { return artifactHash, nil },
		Analyze: func(context.Context) (string, error) {
			analyzeCalls++
			if analyzeCalls == 1 {
				return blockingAnalysis, nil
			}
			return cleanAnalysis, nil
		},
		Remediate: func(_ context.Context, finding speckit.Finding) error {
			remediationCalls++
			if finding.ID != "C1" {
				t.Fatalf("unexpected finding routed: %+v", finding)
			}
			artifactHash = "artifact-1"
			return nil
		},
	}
	analysis, err := RunAnalyzeGate(context.Background(), hooks, DefaultLoopLimits())
	if err != nil || len(BlockingAnalysisFindings(analysis)) != 0 || analyzeCalls != 2 || remediationCalls != 1 {
		t.Fatalf("analysis=%+v err=%v analyzes=%d remediations=%d", analysis, err, analyzeCalls, remediationCalls)
	}
}

func TestRunAnalyzeGateStopsAfterThreeRemediations(t *testing.T) {
	artifactHash, analyzeCalls, remediationCalls := "artifact-0", 0, 0
	hooks := AnalyzeHooks{
		ArtifactHash: func(context.Context) (string, error) { return artifactHash, nil },
		Analyze: func(context.Context) (string, error) {
			analyzeCalls++
			return blockingAnalysis, nil
		},
		Remediate: func(context.Context, speckit.Finding) error {
			remediationCalls++
			artifactHash += "x"
			return nil
		},
	}
	_, err := RunAnalyzeGate(context.Background(), hooks, DefaultLoopLimits())
	if err == nil || !strings.Contains(err.Error(), "ANALYZE_REMEDIATION_EXHAUSTED") || analyzeCalls != 4 || remediationCalls != 3 {
		t.Fatalf("err=%v analyzes=%d remediations=%d", err, analyzeCalls, remediationCalls)
	}
}

func TestRunAnalyzeGateRejectsNoProgressAndAnalysisMutation(t *testing.T) {
	t.Run("no progress", func(t *testing.T) {
		hooks := AnalyzeHooks{
			ArtifactHash: func(context.Context) (string, error) { return "same", nil },
			Analyze:      func(context.Context) (string, error) { return blockingAnalysis, nil },
			Remediate:    func(context.Context, speckit.Finding) error { return nil },
		}
		if _, err := RunAnalyzeGate(context.Background(), hooks, DefaultLoopLimits()); err == nil || !strings.Contains(err.Error(), "ANALYZE_REMEDIATION_NO_PROGRESS") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("read-only violation", func(t *testing.T) {
		hash := "before"
		hooks := AnalyzeHooks{
			ArtifactHash: func(context.Context) (string, error) { return hash, nil },
			Analyze: func(context.Context) (string, error) {
				hash = "changed-during-analysis"
				return cleanAnalysis, nil
			},
			Remediate: func(context.Context, speckit.Finding) error { return nil },
		}
		if _, err := RunAnalyzeGate(context.Background(), hooks, DefaultLoopLimits()); err == nil || !strings.Contains(err.Error(), "ANALYZE_MUTATED_ARTIFACTS") {
			t.Fatalf("error=%v", err)
		}
	})
}
