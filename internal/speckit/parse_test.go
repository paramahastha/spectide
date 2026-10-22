package speckit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAnalysisReportAndSeverityCounts(t *testing.T) {
	output := `## Specification Analysis Report

| ID | Category | Severity | Location(s) | Summary | Recommendation |
|---|---|---|---|---|---|
| C1 | Constitution Alignment | CRITICAL | constitution.md:1 | Ordering conflict | Move analyze gate |
| A2 | Ambiguity | MEDIUM | spec.md:20 | Bound unspecified | Define limits |

### Metrics
- Coverage: 90% (9/10)
`
	got, err := ParseAnalysisOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 2 || got.Critical != 1 || got.Medium != 1 || got.Coverage != "90% (9/10)" {
		t.Fatalf("analysis=%+v", got)
	}
	if _, err = ParseAnalysisOutput("partial output"); err == nil {
		t.Fatal("incomplete report accepted")
	}
}

func TestParseConvergenceReport(t *testing.T) {
	clean, err := ParseConvergenceOutput("✅ Converged — the implementation satisfies the spec, plan, and tasks.")
	if err != nil || !clean.Converged {
		t.Fatalf("result=%+v err=%v", clean, err)
	}
	appended, err := ParseConvergenceOutput("## Convergence Findings\nSummary\nOutcome: tasks_appended\n- [ ] T050 Add evidence per SC-008 (partial)\n")
	if err != nil || len(appended.Appended) != 1 || !strings.HasPrefix(appended.Appended[0], "T050 ") {
		t.Fatalf("result=%+v err=%v", appended, err)
	}
}

func TestValidateConvergenceAppendOnly(t *testing.T) {
	old := "# Tasks\n\n## Phase 2: Foundational\n- [ ] T004 Build foundation\n"
	good := old + "\n## Phase 3: Convergence\n- [ ] T005 Add missing evidence per FR-002 (partial)\n"
	if err := ValidateAppendOnly(old, good); err != nil {
		t.Fatal(err)
	}
	for name, changed := range map[string]string{
		"rewrite": "# changed\n" + strings.TrimPrefix(good, "# Tasks\n"),
		"reuse":   old + "\n## Phase 3: Convergence\n- [ ] T004 Reuse id\n",
		"phase":   old + "\n## Phase 4: Convergence\n- [ ] T005 Add task\n",
		"no_task": old + "\n## Phase 3: Convergence\nNo tasks\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAppendOnly(old, changed); err == nil {
				t.Fatal("invalid convergence append accepted")
			}
		})
	}
}

func TestInspectArtifactsRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "spec.md")); err != nil {
		t.Skip(err)
	}
	if _, err := InspectArtifacts(root); err == nil {
		t.Fatal("artifact symlink escape accepted")
	}
}

func TestDiscoverFeatureDirectoryRequiresOneCompleteProductSpec(t *testing.T) {
	root := t.TempDir()
	feature := filepath.Join(root, "specs", "001-product")
	if err := os.MkdirAll(feature, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
		if err := os.WriteFile(filepath.Join(feature, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := DiscoverFeatureDirectory(root)
	if err != nil || got != feature {
		t.Fatalf("feature=%q err=%v", got, err)
	}
	second := filepath.Join(root, "specs", "002-other")
	if err := os.MkdirAll(second, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "spec.md"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = DiscoverFeatureDirectory(root); err == nil || !strings.Contains(err.Error(), "SPECIFICATION_AMBIGUOUS") {
		t.Fatalf("ambiguous specs error=%v", err)
	}
}
