package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	workspaceguard "github.com/spectide/spectide/internal/workspace"
)

// Engine is deliberately provider-neutral. The built-in fake route is the offline,
// deterministic acceptance path; real providers must be added only after a successful probe.
type Engine struct {
	store   *store.Store
	ownerID string
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	pauses  map[string]chan struct{}
	resumes map[string]chan struct{}
	locks   map[string]string
}

func New(s *store.Store) *Engine {
	return &Engine{store: s, ownerID: uuid.NewString(), cancels: map[string]context.CancelFunc{}, pauses: map[string]chan struct{}{}, resumes: map[string]chan struct{}{}, locks: map[string]string{}}
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
	cfg, err := config.LoadEffective(config.GlobalPath(), config.ProjectPath(workspace))
	if err != nil {
		return model.Run{}, false, err
	}
	if len(cfg.AllowedScopes) > 0 {
		workspace, err = workspaceguard.Canonical(workspace, cfg.AllowedScopes)
		if err != nil {
			return model.Run{}, false, err
		}
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
	if err = e.store.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		return model.Run{}, false, err
	}
	effective := make([]struct {
		Stage     string          `json:"stage"`
		Decision  config.Decision `json:"decision"`
		Simulated bool            `json:"simulated"`
	}, 0, len(stages))
	for _, stage := range stages {
		effective = append(effective, struct {
			Stage     string          `json:"stage"`
			Decision  config.Decision `json:"decision"`
			Simulated bool            `json:"simulated"`
		}{Stage: stage, Decision: decisions[stage], Simulated: true})
	}
	configurationJSON, err := json.Marshal(effective)
	if err != nil {
		return model.Run{}, false, err
	}
	r, reused, err := e.store.CreateRunWithConfiguration(ctx, key, project, prompt, configurationJSON)
	if err != nil {
		return r, false, err
	}
	if reused {
		return r, true, nil
	}
	if err = e.store.AcquireWorkspaceLockOwned(ctx, workspace, r.ID, e.ownerID); err != nil {
		_ = e.store.Event(context.Background(), r.ID, "", "workspace.lock_denied", "warning", map[string]any{"reason": "another run owns this workspace"})
		_ = e.store.SetRunState(context.Background(), r.ID, model.Blocked)
		return r, false, nil
	}
	child, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.cancels[r.ID] = cancel
	e.pauses[r.ID] = make(chan struct{}, 1)
	e.resumes[r.ID] = make(chan struct{}, 1)
	e.locks[r.ID] = workspace
	e.mu.Unlock()
	go e.heartbeat(child, r.ID)
	go e.execute(child, r, decisions)
	return r, false, nil
}

func canonicalWorkspace(project string) (string, error) {
	if project == "" {
		return "", fmt.Errorf("PROJECT_REQUIRED")
	}
	p, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	if resolved, e := filepath.EvalSymlinks(p); e == nil {
		p = resolved
	}
	st, e := os.Stat(p)
	if os.IsNotExist(e) {
		return "", fmt.Errorf("PROJECT_PATH_UNAVAILABLE: %s", p)
	}
	if e != nil {
		return "", fmt.Errorf("PROJECT_PATH_UNAVAILABLE: %w", e)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("PROJECT_NOT_DIRECTORY: %s", p)
	}
	return filepath.Clean(p), nil
}
func (e *Engine) Cancel(ctx context.Context, id string) error {
	e.mu.Lock()
	c := e.cancels[id]
	e.mu.Unlock()
	if c == nil {
		return e.store.RequestRunControl(ctx, id, "cancel")
	}
	c()
	return nil
}

func (e *Engine) Pause(id string) error {
	e.mu.Lock()
	pause := e.pauses[id]
	e.mu.Unlock()
	run, err := e.store.GetRun(context.Background(), id)
	if err != nil {
		return err
	}
	if run.State != model.Running {
		return fmt.Errorf("RUN_CONTROL_NOT_ALLOWED: pause for %s", run.State)
	}
	if pause == nil {
		return e.store.RequestRunControl(context.Background(), id, "pause")
	}
	select {
	case pause <- struct{}{}:
	default:
	}
	return nil
}

func (e *Engine) Resume(id string) error {
	e.mu.Lock()
	resume := e.resumes[id]
	e.mu.Unlock()
	run, err := e.store.GetRun(context.Background(), id)
	if err != nil {
		return err
	}
	if run.State != model.Paused {
		return fmt.Errorf("RUN_CONTROL_NOT_ALLOWED: resume for %s", run.State)
	}
	if resume == nil {
		fresh, err := e.store.RunOwnerFresh(context.Background(), id, time.Now().UTC())
		if err != nil {
			return err
		}
		if !fresh {
			if err = e.store.RecoverAbandonedRuns(context.Background(), time.Now().UTC()); err != nil {
				return err
			}
			return fmt.Errorf("WORKSPACE_DRIFT: owner lease expired; restart resume requires a verified checkpoint and matching workspace, artifact, and Git fingerprints")
		}
		return e.store.RequestRunControl(context.Background(), id, "resume")
	}
	select {
	case resume <- struct{}{}:
	default:
	}
	return nil
}

func (e *Engine) heartbeat(ctx context.Context, runID string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.store.HeartbeatRunOwner(ctx, runID, e.ownerID, time.Now().UTC()); err != nil {
				return
			}
		}
	}
}

func (e *Engine) pauseUntilResume(ctx context.Context, runID, stage string, resume <-chan struct{}) bool {
	if err := e.store.SetRunState(ctx, runID, model.Paused); err != nil {
		return false
	}
	_ = e.store.Event(ctx, runID, stage, "run.paused", "info", map[string]any{"stage": stage})
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-resume:
			_ = e.store.ClearRunControl(ctx, runID)
			return e.markResumed(ctx, runID, stage)
		case <-ticker.C:
			action, err := e.store.RunControl(ctx, runID)
			if err != nil {
				return false
			}
			if action == "cancel" {
				return false
			}
			if action == "resume" {
				_ = e.store.ClearRunControl(ctx, runID)
				return e.markResumed(ctx, runID, stage)
			}
		}
	}
}

func (e *Engine) markResumed(ctx context.Context, runID, stage string) bool {
	if err := e.store.SetRunState(ctx, runID, model.Running); err != nil {
		return false
	}
	_ = e.store.Event(ctx, runID, stage, "run.resumed", "info", map[string]any{"stage": stage})
	return true
}

func (e *Engine) pausePoint(ctx context.Context, runID, stage string, pause, resume <-chan struct{}) bool {
	if action, err := e.store.RunControl(ctx, runID); err != nil {
		return false
	} else if action == "cancel" {
		return false
	} else if action == "pause" {
		if !e.pauseUntilResume(ctx, runID, stage, resume) {
			return false
		}
	}
	select {
	case <-ctx.Done():
		return false
	case <-pause:
		return e.pauseUntilResume(ctx, runID, stage, resume)
	default:
		return true
	}
}

func (e *Engine) cancelRun(runID, stage, stageID string) {
	ctx := context.Background()
	if stageID != "" {
		_ = e.store.SetStage(ctx, stageID, model.Skipped)
	}
	_ = e.store.Event(ctx, runID, stage, "stage.cancelled", "warning", map[string]any{"reason": "cancellation requested"})
	_ = e.store.SetRunState(ctx, runID, model.Cancelled)
}

func (e *Engine) waitStage(ctx context.Context, runID, stage string, duration time.Duration, pause, resume <-chan struct{}) bool {
	deadline := time.Now().Add(duration)
	timer := time.NewTimer(duration)
	defer timer.Stop()
	controlTicker := time.NewTicker(50 * time.Millisecond)
	defer controlTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		case <-pause:
			if !e.pauseUntilResume(ctx, runID, stage, resume) {
				return false
			}
			remaining := time.Until(deadline)
			if remaining < 0 {
				remaining = 0
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(remaining)
			deadline = time.Now().Add(remaining)
		case <-controlTicker.C:
			action, err := e.store.RunControl(ctx, runID)
			if err != nil || action == "cancel" {
				return false
			}
			if action == "pause" {
				if !e.pauseUntilResume(ctx, runID, stage, resume) {
					return false
				}
				remaining := time.Until(deadline)
				if remaining < 0 {
					remaining = 0
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(remaining)
				deadline = time.Now().Add(remaining)
			}
		}
	}
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
	e.mu.Lock()
	pause, resume := e.pauses[r.ID], e.resumes[r.ID]
	e.mu.Unlock()
	_ = e.store.SetRunState(ctx, r.ID, model.Running)
	configuration, err := e.store.RunConfiguration(ctx, r.ID)
	if err != nil {
		_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
		e.finish(r.ID)
		return
	}
	for i, key := range stages {
		select {
		case <-ctx.Done():
			e.cancelRun(r.ID, key, "")
			e.finish(r.ID)
			return
		default:
		}
		if !e.pausePoint(ctx, r.ID, key, pause, resume) {
			if ctx.Err() != nil {
				e.cancelRun(r.ID, key, "")
			} else if action, _ := e.store.RunControl(context.Background(), r.ID); action == "cancel" {
				e.cancelRun(r.ID, key, "")
			}
			e.finish(r.ID)
			return
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
		if !e.waitStage(ctx, r.ID, key, 90*time.Millisecond, pause, resume) {
			if ctx.Err() != nil {
				e.cancelRun(r.ID, key, st.ID)
			} else if action, _ := e.store.RunControl(context.Background(), r.ID); action == "cancel" {
				e.cancelRun(r.ID, key, st.ID)
			}
			e.finish(r.ID)
			return
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
		next := ""
		if i+1 < len(stages) {
			next = stages[i+1]
		}
		state, marshalErr := json.Marshal(map[string]any{"completed_stage": key, "next_stage": next, "configuration_sha256": configuration.SHA256, "cursor_only": true})
		if marshalErr != nil {
			_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
			e.finish(r.ID)
			return
		}
		checkpoint := model.Checkpoint{RunID: r.ID, Stage: key, StateJSON: string(state), WorkspaceFingerprint: workspaceIdentityFingerprint(r.Project)}
		if _, checkpointErr := e.store.SaveCheckpoint(ctx, checkpoint); checkpointErr != nil {
			_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
			e.finish(r.ID)
			return
		}
	}
	for _, g := range []string{"analyze_clean", "required_tests", "converged", "final_review", "simulated_e2e"} {
		_ = e.store.Gate(ctx, r.ID, g, "PASS", "simulated deterministic workflow")
	}
	_ = e.store.SetRunState(ctx, r.ID, model.Completed)
	e.finish(r.ID)
}

// workspaceIdentityFingerprint binds a cursor checkpoint to its canonical
// workspace location. It is not a content-integrity fingerprint and therefore
// is insufficient to authorize restart resume on its own.
func workspaceIdentityFingerprint(project string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(project)))
	return hex.EncodeToString(digest[:])
}

func (e *Engine) finish(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cancel := e.cancels[id]; cancel != nil {
		cancel()
	}
	delete(e.cancels, id)
	delete(e.pauses, id)
	delete(e.resumes, id)
	_ = e.store.ClearRunControl(context.Background(), id)
	if path := e.locks[id]; path != "" {
		_ = e.store.ReleaseWorkspaceLock(context.Background(), path, id)
		delete(e.locks, id)
	}
}
