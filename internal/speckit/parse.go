package speckit

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Finding struct {
	ID         string
	Category   string
	Severity   string
	Location   string
	Summary    string
	Resolution string
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

var analysisRow = regexp.MustCompile(`^\|\s*([A-Z][0-9]+)\s*\|\s*([^|]+)\|\s*(CRITICAL|HIGH|MEDIUM|LOW)\s*\|\s*([^|]+)\|\s*([^|]+)\|\s*([^|]+)\|`)
var convergenceTask = regexp.MustCompile(`^- \[ \] (T[0-9]{3,}) (.+)$`)
var taskID = regexp.MustCompile(`(?m)^- \[[ xX]\] (T[0-9]{3,})\b`)
var phaseHeader = regexp.MustCompile(`(?m)^## Phase ([0-9]+): Convergence\s*$`)
var anyPhaseHeader = regexp.MustCompile(`(?m)^## Phase ([0-9]+):`)

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
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := analysisRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		a.Findings = append(a.Findings, Finding{ID: m[1], Category: strings.TrimSpace(m[2]), Severity: m[3], Location: strings.TrimSpace(m[4]), Summary: strings.TrimSpace(m[5]), Resolution: strings.TrimSpace(m[6])})
		switch m[3] {
		case "CRITICAL":
			a.Critical++
		case "HIGH":
			a.High++
		case "MEDIUM":
			a.Medium++
		case "LOW":
			a.Low++
		}
	}
	if err := scanner.Err(); err != nil {
		return a, err
	}
	a.Complete = strings.Contains(output, "## Specification Analysis Report")
	coverage := regexp.MustCompile(`(?m)^- Coverage:\s*(.+)$`).FindStringSubmatch(output)
	if coverage != nil {
		a.Coverage = strings.TrimSpace(coverage[1])
	}
	if !a.Complete {
		return a, fmt.Errorf("ANALYSIS_REPORT_INCOMPLETE")
	}
	return a, nil
}

func ParseConvergenceOutput(output string) (Convergence, error) {
	c := Convergence{Appended: []string{}}
	if strings.Contains(output, "✅ Converged") {
		c.Converged = true
		return c, nil
	}
	if !strings.Contains(output, "## Convergence Findings") || !strings.Contains(output, "tasks_appended") {
		return c, fmt.Errorf("CONVERGENCE_REPORT_INCOMPLETE")
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		if m := convergenceTask.FindStringSubmatch(strings.TrimSpace(scanner.Text())); m != nil {
			c.Appended = append(c.Appended, m[1]+" "+m[2])
		}
	}
	if err := scanner.Err(); err != nil {
		return c, err
	}
	return c, nil
}

// ValidateAppendOnly proves that the old task ledger is byte-for-byte intact
// and that the new content contains one properly numbered convergence section.
func ValidateAppendOnly(previous, current string) error {
	if !strings.HasPrefix(current, previous) {
		return fmt.Errorf("CONVERGENCE_NOT_APPEND_ONLY")
	}
	appended := strings.TrimPrefix(current, previous)
	if strings.TrimSpace(appended) == "" {
		return fmt.Errorf("CONVERGENCE_APPEND_EMPTY")
	}
	oldIDs := taskID.FindAllStringSubmatch(previous, -1)
	newIDs := taskID.FindAllStringSubmatch(appended, -1)
	if len(newIDs) == 0 {
		return fmt.Errorf("CONVERGENCE_TASKS_MISSING")
	}
	maxID := 0
	seen := make(map[string]bool, len(oldIDs)+len(newIDs))
	for _, item := range oldIDs {
		seen[item[1]] = true
		n, _ := strconv.Atoi(strings.TrimPrefix(item[1], "T"))
		if n > maxID {
			maxID = n
		}
	}
	header := phaseHeader.FindStringSubmatch(appended)
	if header == nil {
		return fmt.Errorf("CONVERGENCE_PHASE_HEADER_MISSING")
	}
	phase, _ := strconv.Atoi(header[1])
	maxPhase := 0
	for _, found := range anyPhaseHeader.FindAllStringSubmatch(previous, -1) {
		n, _ := strconv.Atoi(found[1])
		if n > maxPhase {
			maxPhase = n
		}
	}
	if phase != maxPhase+1 {
		return fmt.Errorf("CONVERGENCE_PHASE_NOT_NEXT: expected %d, got %d", maxPhase+1, phase)
	}
	last := maxID
	for _, item := range newIDs {
		if seen[item[1]] {
			return fmt.Errorf("CONVERGENCE_TASK_ID_REUSED: %s", item[1])
		}
		seen[item[1]] = true
		n, _ := strconv.Atoi(strings.TrimPrefix(item[1], "T"))
		if n <= last {
			return fmt.Errorf("CONVERGENCE_TASK_ID_NOT_INCREASING: %s", item[1])
		}
		last = n
	}
	return nil
}
