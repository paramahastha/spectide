package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/runner"
)

type Verdict string

const (
	Pass    Verdict = "PASS"
	Fail    Verdict = "FAIL"
	Blocked Verdict = "BLOCKED"
	NotRun  Verdict = "NOT_RUN"
)

type Check struct {
	Key               string
	Binary            string
	Args              []string
	Required          bool
	RequireEmptyOut   bool
	Timeout           time.Duration
	UnavailableReason string
}

type Evidence struct {
	Key       string        `json:"key"`
	Verdict   Verdict       `json:"verdict"`
	Command   []string      `json:"command"`
	ProcessID int           `json:"process_id"`
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration_ns"`
	Summary   string        `json:"summary"`
	StartedAt time.Time     `json:"started_at"`
}

type Executor interface {
	Run(context.Context, runner.Invocation) (runner.Result, error)
}

type localExecutor struct{}

func (localExecutor) Run(ctx context.Context, inv runner.Invocation) (runner.Result, error) {
	return runner.Run(ctx, inv)
}

type Registry struct {
	checks   []Check
	executor Executor
	root     string
}

func NewRegistry(executor Executor) *Registry {
	if executor == nil {
		executor = localExecutor{}
	}
	return &Registry{executor: executor}
}

// PinWorkingDirectory binds all checks in this registry to a canonical root
// captured during project discovery. The path is intentionally not resolved
// again when a check runs.
func (r *Registry) PinWorkingDirectory(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("QUALITY_PROJECT_ROOT_NOT_CANONICAL")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.Clean(resolved) != root {
		return fmt.Errorf("QUALITY_PROJECT_ROOT_NOT_CANONICAL")
	}
	r.root = root
	return nil
}

func (r *Registry) WorkingDirectory() string { return r.root }

func (r *Registry) Register(check Check) error {
	if strings.TrimSpace(check.Key) == "" || strings.TrimSpace(check.Binary) == "" {
		return fmt.Errorf("QUALITY_CHECK_KEY_AND_BINARY_REQUIRED")
	}
	for _, current := range r.checks {
		if current.Key == check.Key {
			return fmt.Errorf("QUALITY_CHECK_DUPLICATE: %s", check.Key)
		}
	}
	if check.Timeout <= 0 {
		check.Timeout = 2 * time.Minute
	}
	check.Args = append([]string(nil), check.Args...)
	r.checks = append(r.checks, check)
	return nil
}

func (r *Registry) Checks() []Check {
	return append([]Check(nil), r.checks...)
}

// Except returns the frozen checks excluding keys with already persisted
// results. It is used only by verified checkpoint continuation.
func (r *Registry) Except(completed map[string]struct{}) *Registry {
	filtered := NewRegistry(r.executor)
	filtered.root = r.root
	for _, check := range r.checks {
		if _, skip := completed[check.Key]; !skip {
			check.Args = append([]string(nil), check.Args...)
			_ = filtered.Register(check)
		}
	}
	return filtered
}

func DefaultRegistry(executor Executor) *Registry {
	r := NewRegistry(executor)
	for _, check := range []Check{
		{Key: "format", Binary: "gofmt", Args: []string{"-l", "cmd", "internal"}, Required: true, RequireEmptyOut: true, Timeout: time.Minute},
		{Key: "static_analysis", Binary: "go", Args: []string{"vet", "./..."}, Required: true},
		{Key: "unit_tests", Binary: "go", Args: []string{"test", "./..."}, Required: true},
		{Key: "build", Binary: "go", Args: []string{"build", "-o", filepath.Join(".spectide", "quality-build"), "./cmd/spectide"}, Required: true},
	} {
		_ = r.Register(check)
	}
	return r
}

// Discover returns required, explicit local checks for the project markers
// present at root. Unsupported or incomplete layouts get a visible blocked
// check instead of running SpecTide-specific commands against them.
func Discover(root string, executor Executor) (*Registry, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("QUALITY_PROJECT_ROOT_INVALID")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("QUALITY_PROJECT_ROOT_INVALID")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("QUALITY_PROJECT_ROOT_INVALID")
	}
	registry := NewRegistry(executor)
	registry.root = root
	checks := []Check{}
	_, hasGo, err := readContainedManifest(root, "go.mod")
	if err != nil {
		return nil, err
	}
	if hasGo {
		checks = append(checks,
			Check{Key: "gofmt", Binary: "gofmt", Args: []string{"-l", "."}, Required: true, RequireEmptyOut: true},
			Check{Key: "go_vet", Binary: "go", Args: []string{"vet", "./..."}, Required: true},
			Check{Key: "go_test", Binary: "go", Args: []string{"test", "./..."}, Required: true},
			Check{Key: "go_build", Binary: "go", Args: []string{"build", "./..."}, Required: true},
		)
	}
	packageData, hasPackage, err := readContainedManifest(root, "package.json")
	if err != nil {
		return nil, err
	}
	if hasPackage {
		var packageInfo struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(packageData, &packageInfo) != nil {
			checks = append(checks, Check{Key: "node_project", Binary: "npm", Required: true, UnavailableReason: "package.json is unreadable or invalid"})
		} else if testScript, ok := packageInfo.Scripts["test"]; !ok || strings.TrimSpace(testScript) == "" {
			checks = append(checks, Check{Key: "node_tests", Binary: "npm", Required: true, UnavailableReason: "package.json does not define a test script"})
		} else {
			manager, testArgs := "npm", []string{"test", "--", "--watch=false"}
			_, hasPNPM, manifestErr := readContainedManifest(root, "pnpm-lock.yaml")
			if manifestErr != nil {
				return nil, manifestErr
			}
			_, hasYarn, manifestErr := readContainedManifest(root, "yarn.lock")
			if manifestErr != nil {
				return nil, manifestErr
			}
			if hasPNPM {
				manager = "pnpm"
			} else if hasYarn {
				manager, testArgs = "yarn", []string{"test", "--watch=false"}
			}
			checks = append(checks, Check{Key: "node_tests", Binary: manager, Args: testArgs, Required: true})
			if buildScript, ok := packageInfo.Scripts["build"]; ok && strings.TrimSpace(buildScript) != "" {
				buildArgs := []string{"run", "build"}
				if manager == "yarn" {
					buildArgs = []string{"build"}
				}
				checks = append(checks, Check{Key: "node_build", Binary: manager, Args: buildArgs, Required: true})
			}
		}
	}
	_, hasPyproject, err := readContainedManifest(root, "pyproject.toml")
	if err != nil {
		return nil, err
	}
	_, hasPytest, err := readContainedManifest(root, "pytest.ini")
	if err != nil {
		return nil, err
	}
	if hasPyproject || hasPytest {
		checks = append(checks, Check{Key: "python_tests", Binary: "python3", Args: []string{"-m", "pytest", "-q"}, Required: true})
	}
	if len(checks) == 0 {
		checks = append(checks, Check{Key: "project_quality", Binary: "spectide-unsupported-quality", Required: true, UnavailableReason: "no supported project manifest or quality integration was detected"})
	}
	for _, check := range checks {
		if err := registry.Register(check); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func readContainedManifest(root, relative string) ([]byte, bool, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, false, err
	}
	path := filepath.Join(root, relative)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, false, fmt.Errorf("QUALITY_CHECK_MANIFEST_PATH_INVALID")
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, false, err
	}
	rel, err := filepath.Rel(canonicalRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("QUALITY_CHECK_MANIFEST_PATH_INVALID: %s", relative)
	}
	data, err := os.ReadFile(resolved)
	return data, true, err
}

func (r *Registry) Run(ctx context.Context, root string) []Evidence {
	evidence, _ := r.RunControlled(ctx, root, nil, nil)
	return evidence
}

// RunControlled checks durable run controls between commands and while a
// registered command is active. Pause waits for the caller's boundary hook
// after the current result is persisted; cancel stops the process promptly.
func (r *Registry) RunControlled(ctx context.Context, root string, control func(context.Context) (string, error), after func(context.Context, Evidence) error, inheritedFiles ...*os.File) ([]Evidence, error) {
	results := make([]Evidence, 0, len(r.checks))
	for _, check := range r.checks {
		command := append([]string{check.Binary}, check.Args...)
		if ctx.Err() != nil {
			results = append(results, Evidence{Key: check.Key, Verdict: NotRun, Command: command, Summary: "not started after cancellation"})
			continue
		}
		if control != nil {
			action, err := control(ctx)
			if err != nil {
				return results, err
			}
			if action == "cancel" {
				return results, context.Canceled
			}
		}
		if check.UnavailableReason != "" {
			evidence := Evidence{Key: check.Key, Verdict: Blocked, Command: command, Summary: check.UnavailableReason}
			results = append(results, evidence)
			if after != nil {
				if err := after(ctx, evidence); err != nil {
					return results, err
				}
			}
			continue
		}
		if _, local := r.executor.(localExecutor); local {
			if _, lookErr := exec.LookPath(check.Binary); lookErr != nil {
				evidence := Evidence{Key: check.Key, Verdict: Blocked, Command: command, Summary: "required local command is unavailable"}
				results = append(results, evidence)
				if after != nil {
					if err := after(ctx, evidence); err != nil {
						return results, err
					}
				}
				continue
			}
		}
		started := time.Now().UTC()
		checkCtx, cancel := context.WithTimeout(ctx, check.Timeout)
		monitorDone := make(chan struct{})
		if control != nil {
			go func() {
				defer close(monitorDone)
				ticker := time.NewTicker(50 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-checkCtx.Done():
						return
					case <-ticker.C:
						action, err := control(context.Background())
						if err != nil || action == "cancel" {
							cancel()
							return
						}
					}
				}
			}()
		} else {
			close(monitorDone)
		}
		args := append([]string(nil), check.Args...)
		cleanup := func() {}
		if check.Key == "build" {
			artifact, createErr := os.CreateTemp("", "spectide-quality-build-*")
			if createErr != nil {
				cancel()
				results = append(results, Evidence{Key: check.Key, Verdict: Blocked, Command: append([]string{check.Binary}, check.Args...), Summary: "could not allocate isolated build output"})
				continue
			}
			buildPath := artifact.Name()
			_ = artifact.Close()
			_ = os.Remove(buildPath)
			cleanup = func() { _ = os.Remove(buildPath) }
			if len(args) >= 3 && args[0] == "build" && args[1] == "-o" {
				args[2] = buildPath
			}
		}
		invocation := runner.Invocation{Binary: check.Binary, Args: args, Dir: root, ExtraFiles: inheritedFiles}
		if r.root != "" {
			invocation.ExpectedWorkingDirectory = r.root
		}
		result, err := r.executor.Run(checkCtx, invocation)
		contextErr := checkCtx.Err()
		cancel()
		<-monitorDone
		cleanup()
		evidence := Evidence{Key: check.Key, Verdict: Pass, Command: append([]string{check.Binary}, args...), ProcessID: result.ProcessID, ExitCode: result.ExitCode, Duration: result.Duration, StartedAt: started}
		summary := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
		if len(summary) > 500 {
			summary = summary[:500] + "…"
		}
		evidence.Summary = summary
		switch {
		case contextErr != nil:
			evidence.Verdict = Blocked
			evidence.Summary = "quality check exceeded its configured timeout or was cancelled"
		case result.ExitCode == 126 || result.ExitCode == 127:
			evidence.Verdict = Blocked
			evidence.Summary = "required project command is unavailable"
		case err != nil || result.ExitCode != 0:
			evidence.Verdict = Fail
			if evidence.Summary == "" && err != nil {
				evidence.Summary = err.Error()
			}
		case check.RequireEmptyOut && summary != "":
			evidence.Verdict = Fail
			evidence.Summary = "check reported items requiring correction: " + summary
		}
		results = append(results, evidence)
		if after != nil {
			if err := after(ctx, evidence); err != nil {
				return results, err
			}
		}
	}
	return results, nil
}

func (r *Registry) RequiredPassed(evidence []Evidence) bool {
	verdicts := make(map[string]Verdict, len(evidence))
	for _, item := range evidence {
		verdicts[item.Key] = item.Verdict
	}
	for _, check := range r.checks {
		if check.Required && verdicts[check.Key] != Pass {
			return false
		}
	}
	return true
}
