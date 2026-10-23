package workflow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectide/spectide/internal/speckit"
)

const (
	blockingAnalysis = "## Specification Analysis Report\n\n| ID | Category | Severity | Location(s) | Summary | Recommendation |\n|---|---|---|---|---|---|\n| C1 | Security | CRITICAL | spec.md:1 | Boundary is unsafe | Fix boundary |\n\n### Metrics\n- Coverage: 100% (1/1)\n"
	cleanAnalysis    = "## Specification Analysis Report\n\n| ID | Category | Severity | Location(s) | Summary | Recommendation |\n|---|---|---|---|---|---|\n| I1 | Ambiguity | MEDIUM | spec.md:1 | Bound unspecified | Clarify if needed |\n\n### Metrics\n- Coverage: 100% (1/1)\n"
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
			if finding.ID != "C1" || finding.Owner != "spec.md" {
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

func TestAnalyzeContinuationBoundarySeesSpentRemediationBudget(t *testing.T) {
	artifactHash := "artifact-0"
	analyzeCalls := 0
	budget, err := NewLoopBudget(DefaultLoopLimits())
	if err != nil {
		t.Fatal(err)
	}
	boundaryCalls := 0
	hooks := AnalyzeHooks{
		ArtifactHash: func(context.Context) (string, error) { return artifactHash, nil },
		Analyze: func(context.Context) (string, error) {
			analyzeCalls++
			if analyzeCalls == 1 {
				return blockingAnalysis, nil
			}
			return cleanAnalysis, nil
		},
		Remediate: func(context.Context, speckit.Finding) error { artifactHash = "artifact-1"; return nil },
		Boundary: func(_ context.Context, operation string) error {
			if operation == "continue_after_remediation" {
				boundaryCalls++
				if budget.Counts().AnalyzeRemediations != 1 {
					return fmt.Errorf("checkpoint preceded budget update: %+v", budget.Counts())
				}
			}
			return nil
		},
	}
	if _, err = RunAnalyzeGateWithBudget(context.Background(), hooks, budget, budget.Limits()); err != nil || boundaryCalls != 1 {
		t.Fatalf("analysis err=%v boundary_calls=%d", err, boundaryCalls)
	}
}

func TestReadOnlyAnalysisDetectsSourceAndContractWrites(t *testing.T) {
	for name, relative := range map[string]string{
		"source":   filepath.Join("internal", "service.go"),
		"contract": filepath.Join("specs", "001-product", "contracts", "api.md"),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, relative)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
				t.Fatal(err)
			}
			hooks := AnalyzeHooks{
				ArtifactHash: func(context.Context) (string, error) { return workspaceFingerprint(root) },
				Analyze: func(context.Context) (string, error) {
					if err := os.WriteFile(path, []byte("changed by read-only analyze"), 0o600); err != nil {
						return "", err
					}
					return cleanAnalysis, nil
				},
				Remediate: func(context.Context, speckit.Finding) error { return nil },
			}
			if _, err := RunAnalyzeGate(context.Background(), hooks, DefaultLoopLimits()); err == nil || !strings.Contains(err.Error(), "ANALYZE_MUTATED_ARTIFACTS") {
				t.Fatalf("read-only analyze mutation was accepted: %v", err)
			}
		})
	}
}

func TestReadOnlyAnalysisDetectsMutationsWhenInvocationFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "vendor", "generated.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	hooks := AnalyzeHooks{
		ArtifactHash: func(context.Context) (string, error) { return workspaceFingerprint(root) },
		Analyze: func(context.Context) (string, error) {
			if err := os.WriteFile(path, []byte("changed before failing"), 0o600); err != nil {
				return "", err
			}
			return "", context.Canceled
		},
		Remediate: func(context.Context, speckit.Finding) error { return nil },
	}
	if _, err := RunAnalyzeGate(context.Background(), hooks, DefaultLoopLimits()); err == nil || !strings.Contains(err.Error(), "ANALYZE_MUTATED_ARTIFACTS") {
		t.Fatalf("failed invocation mutation was not preserved as a boundary violation: %v", err)
	}
}

func TestReadOnlyAnalysisRejectsValidReportWhenInvocationAlsoFails(t *testing.T) {
	hooks := AnalyzeHooks{
		ArtifactHash: func(context.Context) (string, error) { return "unchanged", nil },
		Analyze: func(context.Context) (string, error) {
			return cleanAnalysis, fmt.Errorf("provider process failed after emitting a report")
		},
		Remediate: func(context.Context, speckit.Finding) error { return nil },
	}
	if _, err := RunAnalyzeGate(context.Background(), hooks, DefaultLoopLimits()); err == nil || !strings.Contains(err.Error(), "provider process failed") {
		t.Fatalf("failed analyze invocation with valid-looking output was accepted: %v", err)
	}
}

func TestWorkspaceFingerprintIncludesGeneratedDirectoriesAndSymlinks(t *testing.T) {
	for _, relative := range []string{"vendor/generated.go", "dist/app.js", "build/output.bin"} {
		t.Run(relative, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := workspaceFingerprint(root)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte("after"), 0o600); err != nil {
				t.Fatal(err)
			}
			after, err := workspaceFingerprint(root)
			if err != nil || before == after {
				t.Fatalf("generated path mutation was omitted: before=%s after=%s err=%v", before, after, err)
			}
		})
	}
	t.Run("symlink retarget", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"one", "two"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		link := filepath.Join(root, "current")
		if err := os.Symlink("one", link); err != nil {
			t.Fatal(err)
		}
		before, err := workspaceFingerprint(root)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err = os.Symlink("two", link); err != nil {
			t.Fatal(err)
		}
		after, err := workspaceFingerprint(root)
		if err != nil || before == after {
			t.Fatalf("symlink retarget was omitted: before=%s after=%s err=%v", before, after, err)
		}
	})
}

func TestWorkspaceFingerprintIncludesGitState(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.invalid")
	run("config", "user.name", "SpecTide Test")
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "source.go")
	run("commit", "-qm", "initial")
	before, err := workspaceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	run("commit", "--allow-empty", "-qm", "state-only change")
	after, err := workspaceFingerprint(root)
	if err != nil || before == after {
		t.Fatalf("Git state mutation was omitted: before=%s after=%s err=%v", before, after, err)
	}
}

func TestWorkspaceFingerprintExcludesOnlyExistingTaskLedger(t *testing.T) {
	root := t.TempDir()
	tasksPath := filepath.Join(root, "specs", "001-product", "tasks.md")
	if err := os.MkdirAll(filepath.Dir(tasksPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tasksPath, []byte("# Tasks\n- [ ] T001 original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "SpecTide Test"}, {"add", "-A"}, {"commit", "-qm", "initial"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	before, err := workspaceFingerprintExcept(root, []string{"specs/001-product/tasks.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(tasksPath, []byte("# Tasks\n- [ ] T001 original\n- [ ] T002 appended\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterLedgerAppend, err := workspaceFingerprintExcept(root, []string{"specs/001-product/tasks.md"})
	if err != nil || before != afterLedgerAppend {
		t.Fatalf("existing task ledger edit changed the non-ledger fingerprint: before=%s after=%s err=%v", before, afterLedgerAppend, err)
	}
	if err = os.WriteFile(filepath.Join(root, "internal.go"), []byte("package internal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterSourceWrite, err := workspaceFingerprintExcept(root, []string{"specs/001-product/tasks.md"})
	if err != nil || afterSourceWrite == afterLedgerAppend {
		t.Fatalf("non-ledger source addition was not detected: prior=%s after=%s err=%v", afterLedgerAppend, afterSourceWrite, err)
	}
	replacementPath := filepath.Join(root, "specs", "002-replacement", "spec.md")
	if err = os.MkdirAll(filepath.Dir(replacementPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(replacementPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterReplacement, err := workspaceFingerprintExcept(root, []string{"specs/001-product/tasks.md"})
	if err != nil || afterReplacement == afterSourceWrite {
		t.Fatalf("replacement specification was not detected: prior=%s after=%s err=%v", afterSourceWrite, afterReplacement, err)
	}
}

func TestAnalysisOwnerMapsFindingsToTheirOwningArtifact(t *testing.T) {
	for _, test := range []struct {
		finding speckit.Finding
		want    string
	}{
		{speckit.Finding{ID: "S1", Category: "Scope", Summary: "acceptance criterion is missing"}, "spec.md"},
		{speckit.Finding{ID: "P1", Category: "Architecture", Summary: "data model conflict"}, "plan.md"},
		{speckit.Finding{ID: "T1", Category: "Coverage", Location: "tasks.md Phase 2", Summary: "task coverage gap"}, "tasks.md"},
	} {
		got, err := AnalysisOwner(test.finding)
		if err != nil || got != test.want {
			t.Errorf("finding=%+v owner=%q err=%v want=%q", test.finding, got, err, test.want)
		}
	}
	if _, err := AnalysisOwner(speckit.Finding{ID: "unknown", Category: "Ambiguity"}); err == nil || !strings.Contains(err.Error(), "ANALYSIS_OWNER_UNRESOLVED") {
		t.Fatalf("unowned blocking finding error=%v", err)
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
