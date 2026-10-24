package speckit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Finding struct {
	ID         string
	Category   string
	Severity   string
	Location   string
	Summary    string
	Resolution string
	Owner      string
}

type Analysis struct {
	Findings []Finding
	Critical int
	High     int
	Medium   int
	Low      int
	Coverage string
	Complete bool
}

type Convergence struct {
	Converged bool
	Appended  []string
}

type ArtifactState struct {
	SpecPath  string
	PlanPath  string
	TasksPath string
	Present   map[string]bool
}

// DiscoverFeatureDirectory resolves a single existing feature directory. It
// refuses to choose among multiple specs so a workflow cannot silently switch
// the project's product specification.
func DiscoverFeatureDirectory(projectRoot string) (string, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if selected, explicit, selectorErr := selectedFeatureDirectory(root); selectorErr != nil {
		return "", selectorErr
	} else if explicit {
		state, inspectErr := InspectArtifacts(selected)
		if inspectErr != nil {
			return "", inspectErr
		}
		for _, name := range []string{"spec", "plan", "tasks"} {
			if !state.Present[name] {
				return "", fmt.Errorf("SPECIFICATION_ARTIFACT_MISSING: %s", name)
			}
		}
		return selected, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "specs"))
	if err != nil {
		return "", fmt.Errorf("SPECIFICATION_NOT_FOUND: %w", err)
	}
	var featureDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, "specs", entry.Name())
		state, inspectErr := InspectArtifacts(dir)
		if inspectErr != nil {
			return "", inspectErr
		}
		if state.Present["spec"] {
			featureDirs = append(featureDirs, dir)
		}
	}
	if len(featureDirs) == 0 {
		return "", fmt.Errorf("SPECIFICATION_NOT_FOUND: no specs/*/spec.md exists")
	}
	if len(featureDirs) != 1 {
		return "", fmt.Errorf("SPECIFICATION_AMBIGUOUS: found %d product specifications", len(featureDirs))
	}
	state, err := InspectArtifacts(featureDirs[0])
	if err != nil {
		return "", err
	}
	for _, name := range []string{"spec", "plan", "tasks"} {
		if !state.Present[name] {
			return "", fmt.Errorf("SPECIFICATION_ARTIFACT_MISSING: %s", name)
		}
	}
	return featureDirs[0], nil
}

// SelectFeatureDirectory chooses the one existing feature scope, including an
// incomplete scope, or derives a stable initial path for a fresh workspace.
// The selected path is persisted in the workflow checkpoint before execution.
func SelectFeatureDirectory(projectRoot, prompt string) (string, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if selected, explicit, selectorErr := selectedFeatureDirectory(root); selectorErr != nil {
		return "", selectorErr
	} else if explicit {
		return selected, nil
	}
	specsRoot := filepath.Join(root, "specs")
	entries, err := os.ReadDir(specsRoot)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	var artifactDirs, emptyDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(specsRoot, entry.Name())
		state, inspectErr := InspectArtifacts(dir)
		if inspectErr != nil {
			return "", inspectErr
		}
		if state.Present["spec"] || state.Present["plan"] || state.Present["tasks"] {
			artifactDirs = append(artifactDirs, dir)
		} else {
			emptyDirs = append(emptyDirs, dir)
		}
	}
	if len(artifactDirs) > 1 {
		return "", fmt.Errorf("SPECIFICATION_AMBIGUOUS: found %d feature artifact directories", len(artifactDirs))
	}
	if len(artifactDirs) == 1 {
		return artifactDirs[0], nil
	}
	if len(emptyDirs) > 1 {
		return "", fmt.Errorf("SPECIFICATION_AMBIGUOUS: found %d empty feature directories", len(emptyDirs))
	}
	if len(emptyDirs) == 1 {
		return emptyDirs[0], nil
	}
	name := featureDirectoryName(prompt)
	return filepath.Join(specsRoot, name), nil
}

// selectedFeatureDirectory validates Spec-Kit's canonical active-feature
// declaration. Its value is deliberately constrained to a direct, real child
// of specs so a repository-controlled selector cannot widen workflow scope.
func selectedFeatureDirectory(root string) (string, bool, error) {
	selectorPath := filepath.Join(root, ".specify", "feature.json")
	contents, err := os.ReadFile(selectorPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("SPECIFICATION_SELECTOR_UNREADABLE: %w", err)
	}
	var selector struct {
		FeatureDirectory string `json:"feature_directory"`
	}
	if err = json.Unmarshal(contents, &selector); err != nil {
		return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_INVALID: malformed .specify/feature.json")
	}
	value := selector.FeatureDirectory
	if value == "" || filepath.IsAbs(value) || filepath.ToSlash(filepath.Clean(value)) != value {
		return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_INVALID: feature_directory must be canonical specs/<feature>")
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] != "specs" || parts[1] == "" || parts[1] == "." || parts[1] == ".." {
		return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_INVALID: feature_directory must be a direct child of specs")
	}
	specsRoot := filepath.Join(root, "specs")
	target := filepath.Join(root, filepath.FromSlash(value))
	info, statErr := os.Stat(target)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_UNAVAILABLE: selected feature directory does not exist")
		}
		return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_UNAVAILABLE: %w", statErr)
	}
	if !info.IsDir() {
		return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_INVALID: feature_directory is not a directory")
	}
	resolved, resolveErr := filepath.EvalSymlinks(target)
	if resolveErr != nil {
		return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_UNAVAILABLE: %w", resolveErr)
	}
	if filepath.Clean(resolved) != filepath.Clean(target) {
		return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_INVALID: feature_directory must not be a symlink")
	}
	rel, relErr := filepath.Rel(specsRoot, resolved)
	if relErr != nil || rel != parts[1] || filepath.IsAbs(rel) {
		return "", true, fmt.Errorf("SPECIFICATION_SELECTOR_INVALID: feature_directory escapes project specs")
	}
	return resolved, true, nil
}

// EnsureFeatureDirectory creates the selected directory only after workflow
// routing and workspace ownership have been validated.
func EnsureFeatureDirectory(projectRoot, featureDir string) error {
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	absolute, err := canonicalPathAllowMissing(featureDir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, absolute)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Clean(rel) == "." {
		return fmt.Errorf("SPEC_KIT_FEATURE_PATH_ESCAPE")
	}
	if err = os.MkdirAll(absolute, 0o700); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	resolvedRel, err := filepath.Rel(root, resolved)
	if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("SPEC_KIT_FEATURE_PATH_ESCAPE")
	}
	return nil
}

// ValidateUniqueFeatureDirectory verifies that the selected directory remains
// the one active feature scope. An explicit Spec-Kit selector is authoritative;
// without one, legacy workspaces must still have exactly one artifact scope.
func ValidateUniqueFeatureDirectory(projectRoot, featureDir string) error {
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return err
	}
	selected, err := filepath.EvalSymlinks(featureDir)
	if err != nil {
		return err
	}
	selected, err = filepath.Abs(selected)
	if err != nil {
		return err
	}
	if configured, explicit, selectorErr := selectedFeatureDirectory(root); selectorErr != nil {
		return selectorErr
	} else if explicit {
		if configured != selected {
			return fmt.Errorf("SPECIFICATION_SCOPE_NOT_SELECTED")
		}
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "specs"))
	if err != nil {
		return err
	}
	var artifactScopes []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(root, "specs", entry.Name())
		artifacts, inspectErr := InspectArtifacts(directory)
		if inspectErr != nil {
			return inspectErr
		}
		if artifacts.Present["spec"] || artifacts.Present["plan"] || artifacts.Present["tasks"] {
			resolved, resolveErr := filepath.EvalSymlinks(directory)
			if resolveErr != nil {
				return resolveErr
			}
			artifactScopes = append(artifactScopes, resolved)
		}
	}
	if len(artifactScopes) != 1 || artifactScopes[0] != selected {
		return fmt.Errorf("SPECIFICATION_SCOPE_NOT_UNIQUE")
	}
	return nil
}

func canonicalPathAllowMissing(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	current := abs
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func featureDirectoryName(prompt string) string {
	var builder strings.Builder
	separator := false
	for _, r := range strings.ToLower(prompt) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if separator && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			builder.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
		if builder.Len() >= 48 {
			break
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		slug = "product"
	}
	return "001-" + slug
}

var coverageLine = regexp.MustCompile(`^- Coverage(?:\s*%)?:\s*([0-9]+(?:\.[0-9]+)?%)?(?:\s*\(([0-9]+)\s*/\s*([0-9]+)(?:\s+[^)]*)?\))?(?:\s+.+)?$`)
var findingID = regexp.MustCompile(`^[A-Z][A-Z0-9_-]*[0-9]+$`)
var taskLine = regexp.MustCompile(`^- \[([ xX])\] (T[0-9]{3,}[a-z]?)\s+(.+?)\s*$`)
var traceReference = regexp.MustCompile(`(?i)(?:\bFR-[0-9]{3}\b|\bSC-[0-9]{3}\b|\bUS[1-5]\b|\bAC[0-9]+\b|\bconstitution\b|\bplan:\s*[a-z0-9])`)
var planPathReference = regexp.MustCompile(`(?:cmd|internal|web|docs|scripts|specs)/[A-Za-z0-9_./-]+\.(?:go|tsx?|jsx?|py|rs|java|sql|md|ya?ml|json|sh)`)
var phaseLine = regexp.MustCompile(`^## Phase ([0-9]+):\s*(.+?)\s*$`)
var convergenceTask = taskLine

// InspectArtifacts returns the three canonical Spec-Kit files without following
// symlinks outside the feature directory.
func InspectArtifacts(featureDir string) (ArtifactState, error) {
	root, err := filepath.EvalSymlinks(featureDir)
	if err != nil {
		return ArtifactState{}, err
	}
	state := ArtifactState{
		SpecPath: filepath.Join(root, "spec.md"), PlanPath: filepath.Join(root, "plan.md"), TasksPath: filepath.Join(root, "tasks.md"),
		Present: map[string]bool{},
	}
	for name, p := range map[string]string{"spec": state.SpecPath, "plan": state.PlanPath, "tasks": state.TasksPath} {
		resolved, e := filepath.EvalSymlinks(p)
		if e != nil {
			if os.IsNotExist(e) {
				continue
			}
			return state, e
		}
		rel, e := filepath.Rel(root, resolved)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return state, fmt.Errorf("SPEC_KIT_ARTIFACT_PATH_ESCAPE: %s", name)
		}
		info, e := os.Stat(resolved)
		if e != nil {
			return state, e
		}
		state.Present[name] = info.Mode().IsRegular()
	}
	return state, nil
}

func ParseAnalysisOutput(output string) (Analysis, error) {
	a := Analysis{Findings: []Finding{}}
	lines, err := reportLines(output)
	if err != nil {
		return a, err
	}
	headingCount, tableHeader, tableSeparator, coverageCount := 0, false, false, 0
	inFindings := false
	seenIDs := map[string]bool{}
	placeholderNoFindings := false
	for _, line := range lines {
		if line == "## Specification Analysis Report" {
			headingCount++
			continue
		}
		if strings.HasPrefix(line, "## ") && line != "## Specification Analysis Report" && headingCount > 0 {
			break
		}
		if headingCount == 0 {
			continue
		}
		if strings.Contains(line, "Coverage Summary Table") {
			inFindings = false
			continue
		}
		if strings.HasPrefix(line, "|") {
			if tableHeader && !inFindings {
				continue
			}
			cells, ok := markdownCells(line)
			if !ok {
				return a, fmt.Errorf("ANALYSIS_REPORT_MALFORMED_TABLE")
			}
			if !tableHeader {
				if !equalCells(cells, []string{"ID", "Category", "Severity", "Location(s)", "Summary", "Recommendation"}) {
					return a, fmt.Errorf("ANALYSIS_REPORT_UNKNOWN_FINDINGS_TABLE")
				}
				tableHeader = true
				inFindings = true
				continue
			}
			if !tableSeparator {
				if len(cells) != 6 || !isMarkdownSeparator(cells) {
					return a, fmt.Errorf("ANALYSIS_REPORT_MALFORMED_TABLE_SEPARATOR")
				}
				tableSeparator = true
				continue
			}
			if !inFindings {
				return a, fmt.Errorf("ANALYSIS_REPORT_UNEXPECTED_TABLE")
			}
			if isNoFindingPlaceholder(cells) {
				if len(a.Findings) > 0 || placeholderNoFindings {
					return a, fmt.Errorf("ANALYSIS_REPORT_CONTRADICTORY_FINDINGS")
				}
				placeholderNoFindings = true
				continue
			}
			if placeholderNoFindings || len(cells) != 6 {
				return a, fmt.Errorf("ANALYSIS_REPORT_MALFORMED_FINDING_ROW")
			}
			finding := Finding{ID: cells[0], Category: cells[1], Severity: strings.ToUpper(cells[2]), Location: cells[3], Summary: cells[4], Resolution: cells[5]}
			if !findingID.MatchString(finding.ID) || seenIDs[finding.ID] || finding.Category == "" || finding.Location == "" || finding.Summary == "" || finding.Resolution == "" {
				return a, fmt.Errorf("ANALYSIS_REPORT_INVALID_FINDING")
			}
			seenIDs[finding.ID] = true
			switch finding.Severity {
			case "CRITICAL":
				a.Critical++
			case "HIGH":
				a.High++
			case "MEDIUM":
				a.Medium++
			case "LOW":
				a.Low++
			default:
				return a, fmt.Errorf("ANALYSIS_REPORT_UNKNOWN_SEVERITY: %s", finding.Severity)
			}
			a.Findings = append(a.Findings, finding)
			continue
		}
		if strings.HasPrefix(line, "### ") {
			inFindings = false
		}
		if strings.HasPrefix(line, "- Coverage:") || strings.HasPrefix(line, "- Coverage %:") {
			match := coverageLine.FindStringSubmatch(line)
			if match == nil || (match[1] == "" && (match[2] == "" || match[3] == "")) {
				return a, fmt.Errorf("ANALYSIS_REPORT_COVERAGE_INVALID")
			}
			coverageCount++
			a.Coverage = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "- Coverage %:"), "- Coverage:"))
			if match[2] != "" && match[3] != "" {
				numerator, _ := strconv.Atoi(match[2])
				denominator, _ := strconv.Atoi(match[3])
				if denominator == 0 || numerator > denominator {
					return a, fmt.Errorf("ANALYSIS_REPORT_COVERAGE_INVALID")
				}
				if match[1] != "" {
					percent, _ := strconv.ParseFloat(strings.TrimSuffix(match[1], "%"), 64)
					if percent < 0 || percent > 100 || int(percent+0.5) != int((float64(numerator)/float64(denominator))*100+0.5) {
						return a, fmt.Errorf("ANALYSIS_REPORT_COVERAGE_CONTRADICTORY")
					}
				}
			} else if match[1] != "" {
				percent, _ := strconv.ParseFloat(strings.TrimSuffix(match[1], "%"), 64)
				if percent < 0 || percent > 100 {
					return a, fmt.Errorf("ANALYSIS_REPORT_COVERAGE_INVALID")
				}
			}
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "outcome:") {
			outcome := strings.TrimSpace(strings.TrimPrefix(lower, "outcome:"))
			if outcome != "complete" && outcome != "pass" && outcome != "clean" {
				return a, fmt.Errorf("ANALYSIS_REPORT_OUTCOME_CONTRADICTORY")
			}
		}
		if strings.Contains(lower, "analysis incomplete") || strings.Contains(lower, "report truncated") {
			return a, fmt.Errorf("ANALYSIS_REPORT_INCOMPLETE")
		}
	}
	if headingCount != 1 || !tableHeader || !tableSeparator || coverageCount != 1 || (len(a.Findings) == 0 && !placeholderNoFindings && !containsNoFindings(lines)) {
		return a, fmt.Errorf("ANALYSIS_REPORT_INCOMPLETE")
	}
	a.Complete = true
	return a, nil
}

func ParseConvergenceOutput(output string) (Convergence, error) {
	return ParseConvergenceOutputWithPlan(output, "")
}

// ParseConvergenceOutputWithPlan permits a task to cite a concrete code or
// artifact path only when that path appears in the canonical plan.
func ParseConvergenceOutputWithPlan(output, plan string) (Convergence, error) {
	c := Convergence{Appended: []string{}}
	lines, err := reportLines(output)
	if err != nil {
		return c, err
	}
	cleanMarkers, findingsHeaders, outcomeCount := 0, 0, 0
	outcome := ""
	ids := map[string]bool{}
	lastID := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "✅ Converged") {
			cleanMarkers++
		}
		if line == "## Convergence Findings" {
			findingsHeaders++
		}
		if strings.HasPrefix(line, "Outcome:") {
			outcomeCount++
			outcome = strings.TrimSpace(strings.TrimPrefix(line, "Outcome:"))
		}
		if strings.HasPrefix(line, "> ✅ Converged") || strings.HasPrefix(line, "'✅ Converged") || strings.HasPrefix(line, "\"✅ Converged") {
			continue
		}
		if strings.HasPrefix(line, "- [") {
			match := convergenceTask.FindStringSubmatch(line)
			if match == nil || match[1] != " " || !traceableTask(match[3], plan) {
				return c, fmt.Errorf("CONVERGENCE_TASK_INVALID")
			}
			number := taskNumber(match[2])
			if ids[match[2]] || (lastID != 0 && number != lastID+1) {
				return c, fmt.Errorf("CONVERGENCE_TASK_IDS_NOT_CONSECUTIVE")
			}
			ids[match[2]], lastID = true, number
			c.Appended = append(c.Appended, match[2]+" "+strings.TrimSpace(match[3]))
		}
	}
	if cleanMarkers > 0 {
		if cleanMarkers != 1 || findingsHeaders > 1 || outcomeCount != 0 || len(c.Appended) > 0 {
			return c, fmt.Errorf("CONVERGENCE_OUTCOME_CONTRADICTORY")
		}
		c.Converged = true
		return c, nil
	}
	if findingsHeaders != 1 || outcomeCount != 1 || (outcome != "tasks_appended" && outcome != "converged") {
		return c, fmt.Errorf("CONVERGENCE_REPORT_INCOMPLETE")
	}
	if outcome == "tasks_appended" && len(c.Appended) == 0 {
		return c, fmt.Errorf("CONVERGENCE_TASKS_MISSING")
	}
	if outcome == "converged" && len(c.Appended) != 0 {
		return c, fmt.Errorf("CONVERGENCE_OUTCOME_CONTRADICTORY")
	}
	c.Converged = outcome == "converged"
	return c, nil
}

// ValidateAppendOnly proves that the old task ledger is byte-for-byte intact
// and that the new content contains one properly numbered convergence section.
func ValidateAppendOnly(previous, current string) error {
	return ValidateAppendOnlyWithPlan(previous, current, "")
}

// ValidateAppendOnlyWithPlan checks the same strict append contract while
// allowing source-path traceability only when the selected plan names the path.
func ValidateAppendOnlyWithPlan(previous, current, plan string) error {
	if !strings.HasPrefix(current, previous) {
		return fmt.Errorf("CONVERGENCE_NOT_APPEND_ONLY")
	}
	appended := strings.TrimPrefix(current, previous)
	if strings.TrimSpace(appended) == "" {
		return fmt.Errorf("CONVERGENCE_APPEND_EMPTY")
	}
	oldLines, err := ledgerTasks(previous)
	if err != nil {
		return err
	}
	newLines, err := ledgerTasks(appended)
	if err != nil {
		return err
	}
	if len(newLines) == 0 {
		return fmt.Errorf("CONVERGENCE_TASKS_MISSING")
	}
	maxID := 0
	seen := make(map[string]bool, len(oldLines)+len(newLines))
	for _, item := range oldLines {
		if seen[item.id] {
			return fmt.Errorf("CONVERGENCE_TASK_ID_REUSED: %s", item.id)
		}
		seen[item.id] = true
		n := taskNumber(item.id)
		if n > maxID {
			maxID = n
		}
	}
	var phaseNumber int
	phaseCount := 0
	phaseIndex := -1
	appendedLines := strings.Split(appended, "\n")
	for i, line := range appendedLines {
		if match := phaseLine.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			phaseCount++
			phaseIndex = i
			if match[2] != "Convergence" {
				return fmt.Errorf("CONVERGENCE_PHASE_HEADER_INVALID")
			}
			phaseNumber, _ = strconv.Atoi(match[1])
		}
	}
	if phaseCount != 1 {
		return fmt.Errorf("CONVERGENCE_PHASE_HEADER_MISSING")
	}
	maxPhase := 0
	for _, line := range strings.Split(previous, "\n") {
		if found := phaseLine.FindStringSubmatch(strings.TrimSpace(line)); found != nil {
			n, _ := strconv.Atoi(found[1])
			if n > maxPhase {
				maxPhase = n
			}
		}
	}
	if phaseNumber != maxPhase+1 {
		return fmt.Errorf("CONVERGENCE_PHASE_NOT_NEXT: expected %d, got %d", maxPhase+1, phaseNumber)
	}
	for i, line := range appendedLines {
		if strings.HasPrefix(strings.TrimSpace(line), "- [") && i < phaseIndex {
			return fmt.Errorf("CONVERGENCE_TASK_OUTSIDE_PHASE")
		}
	}
	for i, item := range newLines {
		if seen[item.id] {
			return fmt.Errorf("CONVERGENCE_TASK_ID_REUSED: %s", item.id)
		}
		n := taskNumber(item.id)
		if n != maxID+i+1 {
			return fmt.Errorf("CONVERGENCE_TASK_ID_NOT_CONSECUTIVE: %s", item.id)
		}
		if !item.unchecked {
			return fmt.Errorf("CONVERGENCE_TASK_MUST_BE_UNCHECKED: %s", item.id)
		}
		if !traceableTask(item.description, plan) {
			return fmt.Errorf("CONVERGENCE_TASK_NOT_TRACEABLE: %s", item.id)
		}
		seen[item.id] = true
	}
	return nil
}

// AppendedConvergenceTasks returns the unchecked tasks in the newly appended
// ledger section, preserving order for comparison with a convergence report.
func AppendedConvergenceTasks(previous, current string) ([]string, error) {
	return AppendedConvergenceTasksWithPlan(previous, current, "")
}

func AppendedConvergenceTasksWithPlan(previous, current, plan string) ([]string, error) {
	if err := ValidateAppendOnlyWithPlan(previous, current, plan); err != nil {
		return nil, err
	}
	appended := strings.TrimPrefix(current, previous)
	parsed, err := ledgerTasks(appended)
	if err != nil {
		return nil, err
	}
	tasks := make([]string, 0, len(parsed))
	for _, task := range parsed {
		tasks = append(tasks, task.id+" "+task.description)
	}
	return tasks, nil
}

func taskNumber(id string) int {
	digits := strings.TrimPrefix(id, "T")
	for i, r := range digits {
		if r < '0' || r > '9' {
			digits = digits[:i]
			break
		}
	}
	n, _ := strconv.Atoi(digits)
	return n
}

func traceableTask(description, plan string) bool {
	if traceReference.MatchString(description) {
		return true
	}
	if strings.TrimSpace(plan) == "" {
		return false
	}
	paths := planPathReference.FindAllString(description, -1)
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || !strings.Contains(plan, clean) {
			return false
		}
	}
	return true
}

type ledgerTask struct {
	id          string
	unchecked   bool
	description string
}

func ledgerTasks(content string) ([]ledgerTask, error) {
	var tasks []ledgerTask
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		match := taskLine.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("CONVERGENCE_TASK_LINE_MALFORMED")
		}
		tasks = append(tasks, ledgerTask{id: match[2], unchecked: match[1] == " ", description: strings.TrimSpace(match[3])})
	}
	return tasks, nil
}

func reportLines(output string) ([]string, error) {
	if strings.TrimSpace(output) == "" || len(output) > 1<<20 {
		return nil, fmt.Errorf("SPECKIT_REPORT_SIZE_INVALID")
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	lines := make([]string, 0)
	inFence := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if inFence {
		return nil, fmt.Errorf("SPECKIT_REPORT_TRUNCATED_FENCE")
	}
	return lines, nil
}

func markdownCells(line string) ([]string, bool) {
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil, false
	}
	parts := strings.Split(line[1:len(line)-1], "|")
	cells := make([]string, len(parts))
	for i := range parts {
		cells[i] = strings.TrimSpace(parts[i])
	}
	return cells, true
}

func equalCells(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), b[i]) {
			return false
		}
	}
	return true
}

func isMarkdownSeparator(cells []string) bool {
	for _, cell := range cells {
		cell = strings.Trim(cell, ":")
		if len(cell) < 3 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

func isNoFindingPlaceholder(cells []string) bool {
	if len(cells) != 6 {
		return false
	}
	for _, cell := range cells {
		if cell != "—" && cell != "-" && cell != "" {
			return false
		}
	}
	return true
}

func containsNoFindings(lines []string) bool {
	for _, line := range lines {
		if strings.EqualFold(line, "No findings.") || strings.EqualFold(line, "No findings") {
			return true
		}
	}
	return false
}
