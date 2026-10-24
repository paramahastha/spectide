package speckit

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	withSummary, err := ParseConvergenceOutput("## Convergence Findings\n✅ Converged — all requirements satisfy acceptance.\n")
	if err != nil || !withSummary.Converged {
		t.Fatalf("summary plus clean marker=%+v err=%v", withSummary, err)
	}
}

func TestParseInstalledAnalyzeCoverageSummary(t *testing.T) {
	output := `## Specification Analysis Report
| ID | Category | Severity | Location(s) | Summary | Recommendation |
|---|---|---|---|---|---|
| — | — | — | — | — | — |

**Coverage Summary Table:**
| Requirement Key | Has Task? | Task IDs | Notes |
|---|---|---|---|
| FR-001 | Yes | T001 | Covered |

### Metrics
- Total Requirements: 30
- Total Tasks: 114
- Coverage %: 100%
`
	parsed, err := ParseAnalysisOutput(output)
	if err != nil || !parsed.Complete || parsed.Coverage != "100%" {
		t.Fatalf("installed analyze output=%+v err=%v", parsed, err)
	}
}

func TestAnalysisParserRejectsIncompleteMalformedAndContradictoryReports(t *testing.T) {
	valid := `## Specification Analysis Report

| ID | Category | Severity | Location(s) | Summary | Recommendation |
|---|---|---|---|---|---|
| A1 | Coverage | HIGH | spec.md:1 | Missing acceptance evidence | Add evidence per FR-006 |

### Metrics
- Coverage: 90% (9/10)
`
	cases := map[string]string{
		"heading only":                    "## Specification Analysis Report\n",
		"quoted heading":                  "> ## Specification Analysis Report\n" + strings.TrimPrefix(valid, "## Specification Analysis Report\n"),
		"fenced report":                   "```markdown\n" + valid + "```\n",
		"truncated metrics":               strings.Split(valid, "### Metrics")[0],
		"unknown table":                   strings.Replace(valid, "| ID | Category | Severity | Location(s) | Summary | Recommendation |", "| ID | Severity | Finding |", 1),
		"unknown severity":                strings.Replace(valid, "| A1 | Coverage | HIGH |", "| A1 | Coverage | URGENT |", 1),
		"malformed row":                   strings.Replace(valid, "| A1 | Coverage | HIGH | spec.md:1 | Missing acceptance evidence | Add evidence per FR-006 |", "| A1 | Coverage | HIGH | incomplete |", 1),
		"duplicate finding":               strings.Replace(valid, "\n\n### Metrics", "\n| A1 | Coverage | HIGH | spec.md:2 | Duplicate finding | Fix per FR-006 |\n\n### Metrics", 1),
		"contradictory coverage fraction": strings.Replace(valid, "90% (9/10)", "80% (9/10)", 1),
		"outcome failure":                 strings.Replace(valid, "### Metrics", "Outcome: INCOMPLETE\n\n### Metrics", 1),
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseAnalysisOutput(output); err == nil {
				t.Fatal("invalid analysis report was accepted")
			}
		})
	}
	noFindings := `## Specification Analysis Report

| ID | Category | Severity | Location(s) | Summary | Recommendation |
|---|---|---|---|---|---|
| — | — | — | — | — | — |

### Metrics
- Coverage: 100%
`
	parsed, err := ParseAnalysisOutput(noFindings)
	if err != nil || !parsed.Complete || len(parsed.Findings) != 0 {
		t.Fatalf("valid no-findings report=%+v err=%v", parsed, err)
	}
	if _, err = ParseAnalysisOutput(strings.Replace(noFindings, "100%", "100% (38/38 requirements have associated tasks)", 1)); err != nil {
		t.Fatalf("realistic coverage summary was rejected: %v", err)
	}
}

func TestConvergenceParserRejectsQuotedUnknownAndContradictoryOutcomes(t *testing.T) {
	cases := map[string]string{
		"quoted success":            "> ✅ Converged — all gates pass",
		"quoted inline success":     "The command printed \"✅ Converged\" as an example.",
		"fenced success":            "```\n✅ Converged — all gates pass\n```",
		"unknown outcome":           "## Convergence Findings\nOutcome: maybe\n",
		"missing additions":         "## Convergence Findings\nOutcome: tasks_appended\n",
		"contradictory status":      "✅ Converged — all gates pass\n## Convergence Findings\nOutcome: tasks_appended\n- [ ] T050 Add acceptance coverage per FR-006\n",
		"untraceable task":          "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T050 Add coverage\n",
		"reused report id":          "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T050 First item per FR-006\n- [ ] T050 Duplicate item per FR-006\n",
		"nonconsecutive report ids": "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T050 First item per FR-006\n- [ ] T052 Skipped item per FR-006\n",
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConvergenceOutput(output); err == nil {
				t.Fatal("invalid convergence report was accepted")
			}
		})
	}
}

func TestValidateConvergenceAppendOnly(t *testing.T) {
	old := "# Tasks\n\n## Phase 2: Foundational\n- [ ] T004 Build foundation\n"
	good := old + "\n## Phase 3: Convergence\n- [ ] T005 Add missing evidence per FR-002 (partial)\n"
	if err := ValidateAppendOnly(old, good); err != nil {
		t.Fatal(err)
	}
	for name, changed := range map[string]string{
		"rewrite":           "# changed\n" + strings.TrimPrefix(good, "# Tasks\n"),
		"reuse":             old + "\n## Phase 3: Convergence\n- [ ] T004 Reuse id\n",
		"phase":             old + "\n## Phase 4: Convergence\n- [ ] T005 Add task\n",
		"no_task":           old + "\n## Phase 3: Convergence\nNo tasks\n",
		"nonconsecutive":    old + "\n## Phase 3: Convergence\n- [ ] T006 Skip an id per FR-006\n",
		"checked":           old + "\n## Phase 3: Convergence\n- [x] T005 Completed task per FR-006\n",
		"untraceable":       old + "\n## Phase 3: Convergence\n- [ ] T005 Add task without requirement link\n",
		"duplicate old ids": "# Tasks\n\n## Phase 2: Foundational\n- [ ] T004 First task\n- [ ] T004 Reused task\n" + "\n## Phase 3: Convergence\n- [ ] T005 New task per FR-006\n",
		"multiple phases":   old + "\n## Phase 3: Convergence\n- [ ] T005 First task per FR-006\n\n## Phase 4: Another phase\n- [ ] T006 Another task per FR-006\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAppendOnly(old, changed); err == nil {
				t.Fatal("invalid convergence append accepted")
			}
		})
	}
}

func TestMasterLedgerIDsAndConvergencePlanTraceReferences(t *testing.T) {
	master, err := os.ReadFile(filepath.Join("..", "..", "specs", "001-build-spectide", "tasks.md"))
	if err != nil {
		t.Fatal(err)
	}
	ledger := string(master)
	phase, taskID := nextConvergenceAppend(ledger)
	appended := ledger + fmt.Sprintf("\n## Phase %d: Convergence\n- [ ] T%03d Repair the stage cursor per plan: recovery and task traceability (partial)\n", phase, taskID)
	if err = ValidateAppendOnly(ledger, appended); err != nil {
		t.Fatalf("master ledger suffix IDs or plan reference rejected: %v", err)
	}
	if _, err = ParseConvergenceOutput("## Convergence Findings\nOutcome: tasks_appended\n- [ ] T107 Repair stage recovery per plan: recovery and traceability (partial)\n"); err != nil {
		t.Fatalf("installed converge plan source reference rejected: %v", err)
	}
}

func nextConvergenceAppend(ledger string) (int, int) {
	phase, task := 0, 0
	for _, line := range strings.Split(ledger, "\n") {
		if match := phaseLine.FindStringSubmatch(line); match != nil {
			value, _ := strconv.Atoi(match[1])
			if value > phase {
				phase = value
			}
		}
		if match := taskLine.FindStringSubmatch(line); match != nil {
			value := taskNumber(match[2])
			if value > task {
				task = value
			}
		}
	}
	return phase + 1, task + 1
}

func TestNextConvergenceAppendDerivesMultiplePhasesAndSuffixedIDs(t *testing.T) {
	phase, task := nextConvergenceAppend("# Tasks\n## Phase 3: First\n- [x] T009a Historical suffix\n## Phase 8: Convergence\n- [ ] T010 Existing append\n")
	if phase != 9 || task != 11 {
		t.Fatalf("next append = Phase %d/T%03d; want Phase 9/T011", phase, task)
	}
}

func TestPlanFileReferencesRequireCanonicalPlanEvidence(t *testing.T) {
	output := "## Convergence Findings\nOutcome: tasks_appended\n- [ ] T050 Repair the invocation path in internal/workflow/live_stage.go (partial)\n"
	plan := "## Implementation\nUpdate internal/workflow/live_stage.go and verify its stage boundary.\n"
	if _, err := ParseConvergenceOutputWithPlan(output, plan); err != nil {
		t.Fatalf("plan-referenced source path rejected: %v", err)
	}
	if _, err := ParseConvergenceOutputWithPlan(output, "## Implementation\nNo source file cited.\n"); err == nil {
		t.Fatal("source path absent from the canonical plan was accepted")
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
	canonicalRoot, canonicalErr := filepath.EvalSymlinks(root)
	if err != nil || canonicalErr != nil || got != filepath.Join(canonicalRoot, "specs", "001-product") {
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

func TestDiscoverFeatureDirectoryHonorsExplicitActiveFeature(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"005-other", "006-selected"} {
		feature := filepath.Join(root, "specs", directory)
		if err := os.MkdirAll(feature, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
			if err := os.WriteFile(filepath.Join(feature, name), []byte(name), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".specify"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".specify", "feature.json"), []byte(`{"feature_directory":"specs/006-selected"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverFeatureDirectory(root)
	canonicalRoot, canonicalErr := filepath.EvalSymlinks(root)
	if err != nil || canonicalErr != nil || got != filepath.Join(canonicalRoot, "specs", "006-selected") {
		t.Fatalf("selected=%q err=%v", got, err)
	}
}

func TestSelectFeatureDirectorySupportsFreshAndIncompleteScopes(t *testing.T) {
	root := t.TempDir()
	selected, err := SelectFeatureDirectory(root, "Build a safe cache / API")
	canonicalRoot, canonicalErr := filepath.EvalSymlinks(root)
	if err != nil || canonicalErr != nil || selected != filepath.Join(canonicalRoot, "specs", "001-build-a-safe-cache-api") {
		t.Fatalf("fresh feature selection=%q err=%v", selected, err)
	}
	if err = EnsureFeatureDirectory(root, selected); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(selected, "spec.md"), []byte("chosen product scope"), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := SelectFeatureDirectory(root, "a different later prompt")
	if err != nil || again != selected {
		t.Fatalf("existing partial scope was not preserved: selected=%q err=%v", again, err)
	}
	if err = os.WriteFile(filepath.Join(selected, "plan.md"), []byte("existing plan"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(selected, "contracts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if again, err = SelectFeatureDirectory(root, "do not create another scope"); err != nil || again != selected {
		t.Fatalf("existing scope changed after owner artifacts were added: selected=%q err=%v", again, err)
	}
}

func TestSelectFeatureDirectoryHonorsExplicitActiveFeature(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "specs", "006-selected")
	other := filepath.Join(root, "specs", "005-other")
	for _, directory := range []string{selected, other} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "spec.md"), []byte("spec"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".specify"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".specify", "feature.json"), []byte(`{"feature_directory":"specs/006-selected"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := SelectFeatureDirectory(root, "use the selected feature")
	canonicalRoot, canonicalErr := filepath.EvalSymlinks(root)
	if err != nil || canonicalErr != nil || got != filepath.Join(canonicalRoot, "specs", "006-selected") {
		t.Fatalf("selected=%q err=%v", got, err)
	}
	if err = ValidateUniqueFeatureDirectory(root, got); err != nil {
		t.Fatalf("explicit selection was rejected by later validation: %v", err)
	}
}

func TestSelectNextFeatureDirectoryRequiresUnusedSafeSlug(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "specs", "006-completed")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".specify"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".specify", "feature.json"), []byte(`{"feature_directory":"specs/006-completed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := SelectNextFeatureDirectory(root, "007-next-phase")
	canonical, canonicalErr := filepath.EvalSymlinks(root)
	if err != nil || canonicalErr != nil || got != filepath.Join(canonical, "specs", "007-next-phase") {
		t.Fatalf("next scope=%q err=%v", got, err)
	}
	for _, slug := range []string{"../outside", "UPPER", "a", "bad_slug"} {
		if _, err := SelectNextFeatureDirectory(root, slug); err == nil {
			t.Errorf("unsafe slug %q was accepted", slug)
		}
	}
	if _, err := SelectNextFeatureDirectory(root, "006-completed"); err == nil {
		t.Fatal("existing feature slug collision was accepted")
	}
}

func TestSelectFeatureDirectoryRejectsInvalidExplicitSelectors(t *testing.T) {
	cases := map[string]struct {
		selector string
		want     string
	}{
		"malformed JSON":    {selector: `{"feature_directory":`, want: "SPECIFICATION_SELECTOR_INVALID"},
		"missing field":     {selector: `{}`, want: "SPECIFICATION_SELECTOR_INVALID"},
		"path traversal":    {selector: `{"feature_directory":"specs/../outside"}`, want: "SPECIFICATION_SELECTOR_INVALID"},
		"nested scope":      {selector: `{"feature_directory":"specs/006-selected/nested"}`, want: "SPECIFICATION_SELECTOR_INVALID"},
		"missing directory": {selector: `{"feature_directory":"specs/006-missing"}`, want: "SPECIFICATION_SELECTOR_UNAVAILABLE"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".specify"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".specify", "feature.json"), []byte(testCase.selector), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := SelectFeatureDirectory(root, "test"); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error=%v; want %s", err, testCase.want)
			}
		})
	}
}

func TestSelectFeatureDirectoryRejectsExplicitSelectorSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "specs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "specs", "006-selected")); err != nil {
		t.Skip(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".specify"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".specify", "feature.json"), []byte(`{"feature_directory":"specs/006-selected"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectFeatureDirectory(root, "test"); err == nil || !strings.Contains(err.Error(), "SPECIFICATION_SELECTOR_INVALID") {
		t.Fatalf("symlink selector error=%v", err)
	}
}

func TestSelectFeatureDirectoryRetainsNoSelectorAmbiguity(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"001-first", "002-second"} {
		directory := filepath.Join(root, "specs", name)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "tasks.md"), []byte("tasks"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SelectFeatureDirectory(root, "test"); err == nil || !strings.Contains(err.Error(), "SPECIFICATION_AMBIGUOUS") {
		t.Fatalf("no-selector ambiguity error=%v", err)
	}
}

func TestEnsureFeatureDirectoryRejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "specs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "specs", "001-outside")); err != nil {
		t.Skip(err)
	}
	if err := EnsureFeatureDirectory(root, filepath.Join(root, "specs", "001-outside")); err == nil {
		t.Fatal("feature directory symlink escape accepted")
	}
}
