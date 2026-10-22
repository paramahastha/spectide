package workflow

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
)

// Engine is deliberately provider-neutral. The built-in fake route is the offline,
// deterministic acceptance path; real providers must be added only after a successful probe.
type Engine struct {
	store   *store.Store
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	locks   map[string]string
}

func New(s *store.Store) *Engine {
	return &Engine{store: s, cancels: map[string]context.CancelFunc{}, locks: map[string]string{}}
}

var stages = []string{"constitution", "specify", "clarify", "plan", "checklist", "tasks", "analyze", "implement", "tests", "converge", "final_review", "report"}

func (e *Engine) Start(ctx context.Context, project, prompt string) (model.Run, error) {
	r, _, err := e.StartIdempotent(ctx, "", project, prompt)
	return r, err
}

func (e *Engine) StartIdempotent(ctx context.Context, key, project, prompt string) (model.Run, bool, error) {
	workspace, err := canonicalWorkspace(project)
	if err != nil {
		return model.Run{}, false, err
	}
	if project == "" {
		return model.Run{}, false, fmt.Errorf("PROJECT_REQUIRED")
	}
	project = workspace
	if existing, reused, err := e.store.IdempotentRun(ctx, key, project, prompt); err != nil {
		return model.Run{}, false, err
	} else if reused {
		return existing, true, nil
	}
	cfg, err := config.Load(config.ProjectPath(workspace))
	if err != nil {
		return model.Run{}, false, err
	}
	decisions := make(map[string]config.Decision, len(stages))
	for _, stage := range stages {
		decision, e := cfg.Resolve(stage, prompt)
		if e != nil {
			return model.Run{}, false, e
		}
		if decision.Route.Provider != "fake" {
			return model.Run{}, false, fmt.Errorf("LIVE_STAGE_EXECUTION_UNAVAILABLE: %s resolves to %s/%s; run not started", stage, decision.Route.Provider, decision.Route.Profile)
		}
		decisions[stage] = decision
	}
	if err = e.store.RegisterProject(ctx, workspace, path.Base(workspace), []string{workspace}); err != nil {
		return model.Run{}, false, err
	}
	r, reused, err := e.store.CreateRunIdempotent(ctx, key, project, prompt)
	if err != nil {
		return r, false, err
	}
	if reused {
		return r, true, nil
	}
	if err = e.store.AcquireWorkspaceLock(ctx, workspace, r.ID); err != nil {
		_ = e.store.Event(context.Background(), r.ID, "", "workspace.lock_denied", "warning", map[string]any{"reason": "another run owns this workspace"})
		_ = e.store.SetRunState(context.Background(), r.ID, model.Blocked)
		return r, false, nil
	}
	child, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.cancels[r.ID] = cancel
	e.locks[r.ID] = workspace
	e.mu.Unlock()
	go e.execute(child, r, decisions)
	return r, false, nil
}

func canonicalWorkspace(project string) (string, error) {
	p, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	if resolved, e := filepath.EvalSymlinks(p); e == nil {
		p = resolved
	}
	if st, e := os.Stat(p); e == nil && !st.IsDir() {
		return "", fmt.Errorf("PROJECT_NOT_DIRECTORY: %s", p)
	}
	return filepath.Clean(p), nil
}
func (e *Engine) Cancel(ctx context.Context, id string) error {
	e.mu.Lock()
	c := e.cancels[id]
	e.mu.Unlock()
	if c == nil {
		return e.store.SetRunState(ctx, id, model.Cancelled)
	}
	c()
	return nil
}

// Wait is used by foreground CLI runs so the process never closes its state
// store while a simulated workflow still owns it.
func (e *Engine) Wait(ctx context.Context, id string) (model.Run, error) {
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		r, err := e.store.GetRun(ctx, id)
		if err != nil {
			return r, err
		}
		switch r.State {
		case model.Completed, model.Blocked, model.Failed, model.Cancelled:
			return r, nil
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-tick.C:
		}
	}
}

func (e *Engine) execute(ctx context.Context, r model.Run, decisions map[string]config.Decision) {
	_ = e.store.SetRunState(ctx, r.ID, model.Running)
	for i, key := range stages {
		select {
		case <-ctx.Done():
			_ = e.store.Event(context.Background(), r.ID, key, "stage.cancelled", "warning", map[string]any{"reason": "user cancellation"})
			_ = e.store.SetRunState(context.Background(), r.ID, model.Cancelled)
			e.finish(r.ID)
			return
		default:
		}
		d := decisions[key].Route
		st := model.Stage{ID: uuid.NewString(), RunID: r.ID, Key: key, Position: i, State: model.Pending, Provider: d.Provider, Profile: d.Profile, Model: d.Model, Effort: d.Effort, Attempt: 1}
		if err := e.store.AddStage(ctx, st); err != nil {
			_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
			e.finish(r.ID)
			return
		}
		decision := decisions[key]
		_ = e.store.Event(ctx, r.ID, key, "routing.resolved", "info", map[string]any{"provider": d.Provider, "profile": d.Profile, "model": d.Model, "effort": d.Effort, "policy": decision.Explanation, "simulated": true})
		if err := e.store.SaveRoutingDecision(ctx, r.ID, key, d.Provider, d.Profile, d.Model, d.Effort, decision.Mode, decision.Explanation); err != nil {
			_ = e.store.SetStage(context.Background(), st.ID, model.StageFail)
			_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
			e.finish(r.ID)
			return
		}
		_ = e.store.Event(ctx, r.ID, key, "stage.started", "info", map[string]any{"attempt": 1, "read_only": key == "analyze"})
		t := time.NewTimer(90 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			_ = e.store.SetStage(context.Background(), st.ID, model.Skipped)
			_ = e.store.SetRunState(context.Background(), r.ID, model.Cancelled)
			e.finish(r.ID)
			return
		case <-t.C:
		}
		_ = e.store.SetStage(ctx, st.ID, model.Succeeded)
		typ := "stage.completed"
		if key == "analyze" {
			typ = "analysis.clean"
		}
		if key == "converge" {
			typ = "converge.converged"
		}
		if key == "tests" {
			typ = "tests.passed"
		}
		_ = e.store.Event(ctx, r.ID, key, typ, "info", map[string]any{"simulated": true, "evidence": "deterministic fake-provider fixture"})
	}
	for _, g := range []string{"analyze_clean", "required_tests", "converged", "final_review", "simulated_e2e"} {
		_ = e.store.Gate(ctx, r.ID, g, "PASS", "simulated deterministic workflow")
	}
	_ = e.store.SetRunState(ctx, r.ID, model.Completed)
	e.finish(r.ID)
}
func (e *Engine) finish(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.cancels, id)
	if path := e.locks[id]; path != "" {
		_ = e.store.ReleaseWorkspaceLock(context.Background(), path, id)
		delete(e.locks, id)
	}
}
