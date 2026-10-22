package quality

import (
	"context"
	"fmt"
	"os"
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
	Key             string
	Binary          string
	Args            []string
	Required        bool
	RequireEmptyOut bool
	Timeout         time.Duration
}

type Evidence struct {
	Key       string        `json:"key"`
	Verdict   Verdict       `json:"verdict"`
	Command   []string      `json:"command"`
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
}

func NewRegistry(executor Executor) *Registry {
	if executor == nil {
		executor = localExecutor{}
	}
	return &Registry{executor: executor}
}

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

func (r *Registry) Run(ctx context.Context, root string) []Evidence {
	results := make([]Evidence, 0, len(r.checks))
	for _, check := range r.checks {
		if ctx.Err() != nil {
			results = append(results, Evidence{Key: check.Key, Verdict: NotRun, Command: append([]string{check.Binary}, check.Args...), Summary: "not started after cancellation"})
			continue
		}
		started := time.Now().UTC()
		checkCtx, cancel := context.WithTimeout(ctx, check.Timeout)
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
		result, err := r.executor.Run(checkCtx, runner.Invocation{Binary: check.Binary, Args: args, Dir: root})
		contextErr := checkCtx.Err()
		cancel()
		cleanup()
		evidence := Evidence{Key: check.Key, Verdict: Pass, Command: append([]string{check.Binary}, args...), ExitCode: result.ExitCode, Duration: result.Duration, StartedAt: started}
		summary := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
		if len(summary) > 500 {
			summary = summary[:500] + "…"
		}
		evidence.Summary = summary
		switch {
		case contextErr != nil:
			evidence.Verdict = Blocked
			evidence.Summary = "quality check exceeded its configured timeout or was cancelled"
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
	}
	return results
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
