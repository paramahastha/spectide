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
	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/adapters/claude"
	"github.com/spectide/spectide/internal/adapters/codex"
	fakeadapter "github.com/spectide/spectide/internal/adapters/fake"
	"github.com/spectide/spectide/internal/adapters/opencode"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/quality"
	"github.com/spectide/spectide/internal/routing"
	"github.com/spectide/spectide/internal/speckit"
	"github.com/spectide/spectide/internal/store"
	workspaceguard "github.com/spectide/spectide/internal/workspace"
)

// Engine is deliberately provider-neutral. The built-in fake route is the offline,
// deterministic acceptance path; real providers must be added only after a successful probe.
type Engine struct {
	store         *store.Store
	ownerID       string
	adapters      map[adapters.ProviderName]adapters.Adapter
	discover      func(string) (speckit.Installation, error)
	mu            sync.Mutex
	cancels       map[string]context.CancelFunc
	resumes       map[string]chan struct{}
	locks         map[string]string
	processLocks  map[string]*workspaceguard.ProcessLock
	fakeAdapter   fakeadapter.Adapter
	fakeScenarios map[string]fakeadapter.Adapter
}

func New(s *store.Store) *Engine {
	return NewWithAdapters(s, claude.Adapter{}, codex.Adapter{}, opencode.Adapter{})
}

// NewWithAdapters allows deterministic adapter fixtures in workflow tests.
// Production callers should use New, which registers the supported providers.
func NewWithAdapters(s *store.Store, providers ...adapters.Adapter) *Engine {
	registry := make(map[adapters.ProviderName]adapters.Adapter, len(providers))
	for _, provider := range providers {
		if provider != nil {
			registry[provider.Name()] = provider
		}
	}
	return &Engine{store: s, ownerID: uuid.NewString(), adapters: registry, fakeAdapter: fakeadapter.Adapter{Script: []fakeadapter.ScriptStep{{Type: "started", Summary: "simulated stage started"}, {Type: "completed", Summary: "simulated stage completed"}}}, discover: speckit.Discover, cancels: map[string]context.CancelFunc{}, resumes: map[string]chan struct{}{}, locks: map[string]string{}, processLocks: map[string]*workspaceguard.ProcessLock{}}
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
	profiles, err := e.store.Profiles(ctx)
	if err != nil {
		return model.Run{}, false, err
	}
	if err = routing.ValidateProfileRoutes(cfg, workspace, profiles); err != nil {
		return model.Run{}, false, err
	}
	if err = routing.ValidateProfileCapabilities(cfg, profiles); err != nil {
		return model.Run{}, false, err
	}
	decisions := make(map[string]config.Decision, len(stages))
	blockedStage, blockedReason := "", ""
	var installation speckit.Installation
	var installationErr error
	var featureDir string
	for _, stage := range stages {
		decision, resolveErr := cfg.Resolve(stage, prompt)
		if resolveErr != nil {
			return model.Run{}, false, resolveErr
		}
		if decision.Route.Provider == "fake" {
			continue
		}
		installation, installationErr = e.discover(workspace)
		if installationErr == nil {
			featureDir, installationErr = speckit.SelectFeatureDirectory(workspace, prompt)
		}
		break
	}
	for _, stage := range stages {
		decision, resolveErr := cfg.Resolve(stage, prompt)
		if resolveErr != nil {
			return model.Run{}, false, resolveErr
		}
		if decision.Route.Provider != "fake" && blockedStage == "" {
			if _, found := e.adapters[adapters.ProviderName(decision.Route.Provider)]; !found {
				blockedStage, blockedReason = stage, "PROVIDER_ADAPTER_UNAVAILABLE: "+decision.Route.Provider
			} else if !officialStage(stage) {
				blockedStage, blockedReason = stage, "LIVE_STAGE_UNSUPPORTED: no verified Spec-Kit command for "+stage
			} else if installationErr != nil {
				blockedStage, blockedReason = stage, installationErr.Error()
			} else if !installation.Verified {
				blockedStage, blockedReason = stage, "SPEC_KIT_INTEGRATION_UNVERIFIED"
			} else if _, requireErr := speckit.RequireStage(installation, liveSpecKitStage(stage)); requireErr != nil {
				blockedStage, blockedReason = stage, requireErr.Error()
			} else if featureDir == "" {
				blockedStage, blockedReason = stage, "SPECIFICATION_NOT_FOUND"
			}
		}
		decisions[stage] = decision
	}
	if err = e.store.RegisterProject(ctx, workspace, path.Base(workspace), []string{workspace}); err != nil {
		return model.Run{}, false, err
	}
	if err = e.store.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		return model.Run{}, false, err
	}
	qualityRegistry, qualityErr := quality.Discover(workspace, nil)
	if qualityErr != nil {
		return model.Run{}, false, qualityErr
	}
	effective := buildEffectiveRunConfiguration(cfg, decisions, profiles, qualityRegistry)
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
	if blockedStage != "" {
		blocked, blockErr := e.blockUnavailableStage(ctx, r, blockedStage, decisions[blockedStage], blockedReason)
		return blocked, false, blockErr
	}
	processLock, lockErr := workspaceguard.AcquireProcessLock(workspace)
	if lockErr != nil {
		_ = e.store.Event(context.Background(), r.ID, "", "workspace.lock_denied", "warning", map[string]any{"reason": "another process owns this canonical workspace"})
		_ = e.store.SetRunState(context.Background(), r.ID, model.Blocked)
		return r, false, nil
	}
	if err = e.store.AcquireWorkspaceLockOwned(ctx, workspace, r.ID, e.ownerID); err != nil {
		_ = processLock.Close()
		_ = e.store.Event(context.Background(), r.ID, "", "workspace.lock_denied", "warning", map[string]any{"reason": "another run owns this workspace"})
		_ = e.store.SetRunState(context.Background(), r.ID, model.Blocked)
		return r, false, nil
	}
	if featureDir != "" {
		if err = speckit.EnsureFeatureDirectory(workspace, featureDir); err != nil {
			_ = e.store.Event(context.Background(), r.ID, "", "feature.directory_failed", "error", map[string]any{"reason": safeStageError(err)})
			_ = e.store.SetRunState(context.Background(), r.ID, model.Blocked)
			_ = e.store.ReleaseWorkspaceLock(context.Background(), workspace, r.ID)
			_ = processLock.Close()
			return r, false, err
		}
	}
	configuration, err := e.store.RunConfiguration(ctx, r.ID)
	if err == nil {
		err = e.saveCursorCheckpoint(ctx, r, featureDir, "starting", stages[0], configuration.SHA256, "LOCAL", LoopLimits{})
	}
	if err != nil {
		_ = e.store.Event(context.Background(), r.ID, "", "checkpoint.failed", "error", map[string]any{"reason": safeStageError(err)})
		_ = e.store.SetRunState(context.Background(), r.ID, model.Blocked)
		_ = e.store.ReleaseWorkspaceLock(context.Background(), workspace, r.ID)
		_ = processLock.Close()
		return r, false, err
	}
	child, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.cancels[r.ID] = cancel
	e.resumes[r.ID] = make(chan struct{}, 1)
	e.locks[r.ID] = workspace
	e.processLocks[r.ID] = processLock
	e.mu.Unlock()
	go e.heartbeat(child, r.ID, cancel)
	go e.execute(child, r, decisions, installation, featureDir, 0, LoopLimits{}, nil)
	return r, false, nil
}

func officialStage(stage string) bool {
	switch stage {
	case "constitution", "specify", "clarify", "plan", "checklist", "tasks", "analyze", "implement", "converge", "final_review":
		return true
	default:
		return false
	}
}

func liveSpecKitStage(stage string) string {
	if stage == "final_review" {
		return "analyze"
	}
	return stage
}

func (e *Engine) blockUnavailableStage(ctx context.Context, run model.Run, stage string, decision config.Decision, reason string) (model.Run, error) {
	route := decision.Route
	attempt := model.Stage{ID: uuid.NewString(), RunID: run.ID, Key: stage, Position: indexOfStage(stage), State: model.Pending, Provider: route.Provider, Profile: route.Profile, Model: route.Model, Effort: route.Effort, Attempt: 1}
	if err := e.store.AddStage(ctx, attempt); err != nil {
		return model.Run{}, err
	}
	if err := e.store.SaveRoutingDecision(ctx, run.ID, stage, route.Provider, route.Profile, route.Model, route.Effort, decision.Mode, decision.Explanation, route.Provider == "fake"); err != nil {
		return model.Run{}, err
	}
	if err := e.store.Event(ctx, run.ID, stage, "stage.blocked", "warning", map[string]any{"reason": reason, "execution": "NOT_RUN"}); err != nil {
		return model.Run{}, err
	}
	if err := e.store.SetStage(ctx, attempt.ID, model.Skipped); err != nil {
		return model.Run{}, err
	}
	if err := e.store.Gate(ctx, run.ID, "stage_execution_available", "BLOCKED", reason); err != nil {
		return model.Run{}, err
	}
	if err := e.store.SetRunState(ctx, run.ID, model.Blocked); err != nil {
		return model.Run{}, err
	}
	return e.store.GetRun(ctx, run.ID)
}

func indexOfStage(stage string) int {
	for i, key := range stages {
		if key == stage {
			return i
		}
	}
	return -1
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
	run, err := e.store.GetRun(context.Background(), id)
	if err != nil {
		return err
	}
	if run.State != model.Running {
		return fmt.Errorf("RUN_CONTROL_NOT_ALLOWED: pause for %s", run.State)
	}
	return e.store.RequestRunControl(context.Background(), id, "pause")
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
	canonical, canonicalErr := canonicalWorkspace(run.Project)
	if canonicalErr != nil || canonical != run.Project {
		return e.blockUnverifiedRestart(run, fmt.Errorf("WORKSPACE_DRIFT: canonical workspace identity changed"))
	}
	if err = e.store.VerifyRunWorkspaceLock(context.Background(), canonical, id); err != nil {
		return e.blockUnverifiedRestart(run, err)
	}
	configuration, err := e.store.RunConfiguration(context.Background(), id)
	if err != nil {
		return e.blockUnverifiedRestart(run, err)
	}
	_, checkpointState, err := e.verifyRestartCheckpoint(context.Background(), run, configuration)
	if err != nil {
		return e.blockUnverifiedRestart(run, err)
	}
	pausedStage, err := e.latestPausedStage(id)
	if err != nil || pausedStage == "" || pausedStage != checkpointState.NextStage {
		return e.blockUnverifiedRestart(run, fmt.Errorf("WORKSPACE_DRIFT: paused stage does not match checkpoint cursor"))
	}
	fresh, err := e.store.RunOwnerFresh(context.Background(), id, time.Now().UTC())
	if err != nil {
		return e.blockUnverifiedRestart(run, err)
	}
	if fresh {
		if resume == nil {
			return e.store.RequestRunControl(context.Background(), id, "resume")
		}
		select {
		case resume <- struct{}{}:
		default:
		}
		return nil
	}
	if resume != nil {
		return e.blockUnverifiedRestart(run, fmt.Errorf("WORKSPACE_DRIFT: owner lease expired while local worker remains registered"))
	}
	{
		effectiveConfiguration, parseErr := parseEffectiveRunConfiguration(configuration.ConfigJSON)
		if parseErr != nil {
			return e.blockUnverifiedRestart(run, parseErr)
		}
		decisions := effectiveConfiguration.decisions()
		featureDir := ""
		if checkpointState.FeatureDirectory != "" {
			featureDir = filepath.Join(run.Project, filepath.FromSlash(checkpointState.FeatureDirectory))
		}
		installation, featureDir, discoverErr := e.resumeSpecKit(run.Project, decisions, featureDir)
		if discoverErr != nil {
			return e.blockUnverifiedRestart(run, discoverErr)
		}
		budget, budgetErr := restoreLoopBudget(effectiveConfiguration.loopLimits(), checkpointState.LoopCounts)
		if budgetErr != nil {
			return e.blockUnverifiedRestart(run, budgetErr)
		}
		processLock, lockErr := workspaceguard.AcquireProcessLock(canonical)
		if lockErr != nil {
			return fmt.Errorf("WORKSPACE_OWNER_ACTIVE: %w", lockErr)
		}
		claimed := false
		defer func() {
			if !claimed {
				_ = processLock.Close()
			}
		}()
		if err = e.store.ClaimExpiredPausedRun(context.Background(), id, canonical, e.ownerID, time.Now().UTC()); err != nil {
			return err
		}
		if _, _, err = e.verifyRestartCheckpoint(context.Background(), run, configuration); err != nil {
			return e.blockUnverifiedRestart(run, err)
		}
		if err = e.store.ClearRunControl(context.Background(), id); err != nil {
			_ = e.store.ReleaseWorkspaceLock(context.Background(), canonical, id)
			return err
		}
		gates, gateErr := e.store.Gates(context.Background(), id)
		if gateErr != nil {
			_ = e.store.ReleaseWorkspaceLock(context.Background(), canonical, id)
			return gateErr
		}
		gateVerdicts := make(map[string]string, len(gates))
		for _, gate := range gates {
			if gate.Verdict == "PASS" {
				gateVerdicts[gate.Key] = gate.Verdict
			}
		}
		startAt := indexOfStage(checkpointState.NextStage)
		if startAt < 0 {
			_ = e.store.ReleaseWorkspaceLock(context.Background(), canonical, id)
			return e.blockUnverifiedRestart(run, fmt.Errorf("WORKSPACE_DRIFT: checkpoint stage cursor is invalid"))
		}
		child, cancel := context.WithCancel(context.Background())
		e.mu.Lock()
		e.cancels[id] = cancel
		e.resumes[id] = make(chan struct{}, 1)
		e.locks[id] = canonical
		e.processLocks[id] = processLock
		e.mu.Unlock()
		claimed = true
		go e.heartbeat(child, id, cancel)
		go e.execute(child, run, decisions, installation, featureDir, startAt, budget.Counts(), gateVerdicts)
		return nil
	}
}

func (e *Engine) blockUnverifiedRestart(run model.Run, cause error) error {
	e.mu.Lock()
	cancel := e.cancels[run.ID]
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if err := e.store.BlockPausedRunForWorkspaceDrift(context.Background(), run.ID, cause.Error()); err != nil {
		return fmt.Errorf("WORKSPACE_DRIFT: restart resume denied: %v (failed to persist drift block: %w)", cause, err)
	}
	return fmt.Errorf("WORKSPACE_DRIFT: restart resume denied: %w", cause)
}

func (e *Engine) latestPausedStage(runID string) (string, error) {
	events, err := e.store.Events(context.Background(), runID, 0)
	if err != nil {
		return "", err
	}
	stage := ""
	for _, event := range events {
		switch event.Type {
		case "run.resumed":
			stage = ""
		case "run.paused":
			if value, ok := event.Payload["stage"].(string); ok {
				stage = value
			}
		}
	}
	return stage, nil
}

func (e *Engine) resumeSpecKit(project string, decisions map[string]config.Decision, featureDir string) (speckit.Installation, string, error) {
	for _, stage := range stages {
		if decisions[stage].Route.Provider == "fake" {
			continue
		}
		if _, available := e.adapters[adapters.ProviderName(decisions[stage].Route.Provider)]; !available {
			return speckit.Installation{}, "", fmt.Errorf("PROVIDER_ADAPTER_UNAVAILABLE: %s", decisions[stage].Route.Provider)
		}
		installation, err := e.discover(project)
		if err != nil {
			return speckit.Installation{}, "", err
		}
		if !installation.Verified {
			return speckit.Installation{}, "", fmt.Errorf("SPEC_KIT_INTEGRATION_UNVERIFIED")
		}
		if featureDir == "" {
			return speckit.Installation{}, "", fmt.Errorf("WORKSPACE_DRIFT: checkpoint has no selected feature directory")
		}
		if err = speckit.EnsureFeatureDirectory(project, featureDir); err != nil {
			return speckit.Installation{}, "", fmt.Errorf("WORKSPACE_DRIFT: selected feature directory is unavailable")
		}
		for _, routedStage := range stages {
			if decisions[routedStage].Route.Provider == "fake" || !officialStage(routedStage) {
				continue
			}
			if _, err = speckit.RequireStage(installation, liveSpecKitStage(routedStage)); err != nil {
				return speckit.Installation{}, "", err
			}
		}
		return installation, featureDir, nil
	}
	return speckit.Installation{}, "", nil
}

func (e *Engine) heartbeat(ctx context.Context, runID string, cancel context.CancelFunc) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.store.HeartbeatRunOwner(ctx, runID, e.ownerID, time.Now().UTC()); err != nil {
				cancel()
				return
			}
		}
	}
}

func (e *Engine) pauseUntilResume(ctx context.Context, runID, stage string, resume <-chan struct{}) bool {
	if err := e.store.SetRunState(ctx, runID, model.Paused); err != nil {
		return false
	}
	if err := e.store.Event(ctx, runID, stage, "run.paused", "info", map[string]any{"stage": stage}); err != nil {
		_ = e.store.SetRunState(context.Background(), runID, model.Failed)
		return false
	}
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
	if err := e.store.Event(ctx, runID, stage, "run.resumed", "info", map[string]any{"stage": stage}); err != nil {
		_ = e.store.SetRunState(context.Background(), runID, model.Failed)
		return false
	}
	return true
}

func (e *Engine) pausePoint(ctx context.Context, runID, stage string, resume <-chan struct{}) bool {
	if action, err := e.store.RunControl(ctx, runID); err != nil {
		return false
	} else if action == "cancel" {
		return false
	} else if action == "pause" {
		if !e.pauseUntilResume(ctx, runID, stage, resume) {
			return false
		}
	}
	return ctx.Err() == nil
}

// loopControlBoundary is called after each inner provider/check invocation has
// persisted its result. The checkpoint points back to the active workflow
// stage so a process restart can safely replay that stage from its artifacts.
func (e *Engine) loopControlBoundary(ctx context.Context, run model.Run, stage, featureDir string, budget *LoopBudget) error {
	action, err := e.store.RunControl(ctx, run.ID)
	if err != nil {
		return err
	}
	if action == "cancel" {
		return context.Canceled
	}
	if action != "pause" {
		return ctx.Err()
	}
	configuration, err := e.store.RunConfiguration(ctx, run.ID)
	if err != nil {
		return err
	}
	stageIndex := indexOfStage(stage)
	if stageIndex < 0 {
		return fmt.Errorf("WORKFLOW_STAGE_UNKNOWN: %s", stage)
	}
	checkpointStage := "starting"
	if stageIndex > 0 {
		checkpointStage = stages[stageIndex-1]
	}
	if err = e.saveCursorCheckpoint(ctx, run, featureDir, checkpointStage, stage, configuration.SHA256, "LIVE", budget.Counts()); err != nil {
		return err
	}
	e.mu.Lock()
	resume := e.resumes[run.ID]
	e.mu.Unlock()
	if !e.pauseUntilResume(ctx, run.ID, stage, resume) {
		return context.Canceled
	}
	return ctx.Err()
}

func (e *Engine) cancelRun(runID, stage, stageID string) error {
	ctx := context.Background()
	if run, err := e.store.GetRun(ctx, runID); err == nil && run.State == model.Blocked {
		return nil
	}
	if stageID != "" {
		if err := e.store.SetStage(ctx, stageID, model.Skipped); err != nil {
			return err
		}
	}
	if err := e.store.Event(ctx, runID, stage, "stage.cancelled", "warning", map[string]any{"reason": "cancellation requested"}); err != nil {
		return err
	}
	return e.store.SetRunState(ctx, runID, model.Cancelled)
}

func (e *Engine) failPersistence(runID string) {
	_ = e.store.SetRunState(context.Background(), runID, model.Failed)
	e.finish(runID)
}

func (e *Engine) waitStage(ctx context.Context, runID string, duration time.Duration) bool {
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
		case <-controlTicker.C:
			action, err := e.store.RunControl(ctx, runID)
			if err != nil || action == "cancel" {
				return false
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

func (e *Engine) execute(ctx context.Context, r model.Run, decisions map[string]config.Decision, installation speckit.Installation, featureDir string, startAt int, restoredCounts LoopLimits, restoredGates map[string]string) {
	e.mu.Lock()
	resume := e.resumes[r.ID]
	e.mu.Unlock()
	_ = e.store.SetRunState(ctx, r.ID, model.Running)
	configuration, err := e.store.RunConfiguration(ctx, r.ID)
	if err != nil {
		_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
		e.finish(r.ID)
		return
	}
	effectiveConfiguration, err := parseEffectiveRunConfiguration(configuration.ConfigJSON)
	if err != nil {
		_ = e.store.SetRunState(context.Background(), r.ID, model.Blocked)
		e.finish(r.ID)
		return
	}
	decisions = effectiveConfiguration.decisions()
	loopBudget, err := restoreLoopBudget(effectiveConfiguration.loopLimits(), restoredCounts)
	if err != nil {
		_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
		e.finish(r.ID)
		return
	}
	hasLiveStages := false
	runGateVerdicts := make(map[string]string)
	for key, verdict := range restoredGates {
		runGateVerdicts[key] = verdict
	}
	for _, decision := range decisions {
		if decision.Route.Provider != "fake" {
			hasLiveStages = true
			break
		}
	}
	for i := startAt; i < len(stages); i++ {
		key := stages[i]
		select {
		case <-ctx.Done():
			if err := e.cancelRun(r.ID, key, ""); err != nil {
				e.failPersistence(r.ID)
			} else {
				e.finish(r.ID)
			}
			return
		default:
		}
		if !e.pausePoint(ctx, r.ID, key, resume) {
			if ctx.Err() != nil {
				if err := e.cancelRun(r.ID, key, ""); err != nil {
					e.failPersistence(r.ID)
					return
				}
			} else if action, _ := e.store.RunControl(context.Background(), r.ID); action == "cancel" {
				if err := e.cancelRun(r.ID, key, ""); err != nil {
					e.failPersistence(r.ID)
					return
				}
			}
			e.finish(r.ID)
			return
		}
		d := decisions[key].Route
		st := model.Stage{ID: uuid.NewString(), RunID: r.ID, Key: key, Position: i, State: model.Pending, Provider: d.Provider, Profile: d.Profile, Model: d.Model, Effort: d.Effort, Attempt: 1}
		if err := e.store.AddStage(ctx, st); err != nil {
			e.failPersistence(r.ID)
			return
		}
		decision := decisions[key]
		stageSimulated := d.Provider == "fake" && !(key == "tests" && hasLiveStages)
		if err := e.store.SaveRoutingDecision(ctx, r.ID, key, d.Provider, d.Profile, d.Model, d.Effort, decision.Mode, decision.Explanation, stageSimulated); err != nil {
			e.failPersistence(r.ID)
			return
		}
		if err := e.store.Event(ctx, r.ID, key, "stage.started", "info", map[string]any{"attempt": 1, "read_only": key == "analyze"}); err != nil {
			e.failPersistence(r.ID)
			return
		}
		if key == "implement" && d.Provider != "fake" {
			action := fmt.Sprintf("Write project files during implement stage via %s/%s", d.Provider, d.Profile)
			approval, approvalErr := e.store.CreateWorkflowApprovalAndPause(ctx, r.ID, key, action)
			if approvalErr != nil {
				e.failPersistence(r.ID)
				return
			}
			if approval.Status == model.ApprovalPending {
				approved, waitErr := e.waitForSensitiveApproval(ctx, r.ID, key, st.ID, approval, resume)
				if waitErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if !approved {
					e.finish(r.ID)
					return
				}
			} else if approval.Status == model.ApprovalDenied {
				if err := e.denySensitiveApproval(ctx, r.ID, key, st.ID, approval); err != nil {
					e.failPersistence(r.ID)
				} else {
					e.finish(r.ID)
				}
				return
			}
		}
		if d.Provider != "fake" {
			ownerPath, present, inspectErr := existingOwnerArtifact(r.Project, featureDir, key)
			if inspectErr != nil {
				if eventErr := e.store.Event(context.Background(), r.ID, key, "stage.owner_artifact_invalid", "error", map[string]any{"reason": safeStageError(inspectErr)}); eventErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if stageErr := e.store.SetStage(context.Background(), st.ID, model.StageFail); stageErr != nil {
					e.failPersistence(r.ID)
					return
				}
				_ = e.store.SetRunState(context.Background(), r.ID, model.Blocked)
				e.finish(r.ID)
				return
			}
			if present {
				if err := e.store.SetStage(ctx, st.ID, model.Skipped); err != nil {
					e.failPersistence(r.ID)
					return
				}
				if err := e.store.Event(ctx, r.ID, key, "stage.owner_artifact_present", "info", map[string]any{"path": ownerPath, "execution": "NOT_RUN"}); err != nil {
					e.failPersistence(r.ID)
					return
				}
				next := ""
				if i+1 < len(stages) {
					next = stages[i+1]
				}
				if err := e.saveCursorCheckpoint(ctx, r, featureDir, key, next, configuration.SHA256, "PRESERVED", loopBudget.Counts()); err != nil {
					e.failPersistence(r.ID)
					return
				}
				continue
			}
		}
		if key == "converge" && hasLiveStages && d.Provider != "fake" {
			if err := e.store.SetStage(ctx, st.ID, model.StageRun); err != nil {
				e.failPersistence(r.ID)
				return
			}
			err := e.runLiveConvergenceLoop(ctx, r, decisions, installation, featureDir, runGateVerdicts, loopBudget)
			if err != nil {
				if persistErr := e.failLiveQualityStage(ctx, r.ID, key, st.ID, "converged", err, ctx); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				e.finish(r.ID)
				return
			}
			runGateVerdicts["converged"] = "PASS"
			next := ""
			if i+1 < len(stages) {
				next = stages[i+1]
			}
			if err := e.completeLiveQualityStage(ctx, r.ID, st.ID, key, next, configuration.SHA256, "converged", "bounded append-only convergence completed with passing required gates", loopBudget, featureDir); err != nil {
				_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
				e.finish(r.ID)
				return
			}
			continue
		}
		if key == "analyze" && hasLiveStages && d.Provider != "fake" {
			if err := e.store.SetStage(ctx, st.ID, model.StageRun); err != nil {
				e.failPersistence(r.ID)
				return
			}
			analysis, analyzeErr := e.runLiveAnalyzeLoop(ctx, r, decisions, installation, featureDir, loopBudget)
			if analyzeErr != nil {
				if ctx.Err() != nil {
					if persistErr := e.cancelRun(r.ID, key, st.ID); persistErr != nil {
						e.failPersistence(r.ID)
					} else {
						e.finish(r.ID)
					}
					return
				}
				if persistErr := e.store.SetStage(context.Background(), st.ID, model.StageFail); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if persistErr := e.store.Gate(context.Background(), r.ID, "analyze_clean", "FAIL", safeStageError(analyzeErr)); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if persistErr := e.store.Event(context.Background(), r.ID, key, "analysis.failed", "error", map[string]any{"reason": safeStageError(analyzeErr), "execution": "LIVE"}); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if persistErr := e.store.SetRunState(context.Background(), r.ID, model.Blocked); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				e.finish(r.ID)
				return
			}
			runGateVerdicts["analyze_clean"] = "PASS"
			if err := e.store.Gate(ctx, r.ID, "analyze_clean", "PASS", fmt.Sprintf("read-only analysis passed after %d blocking findings", len(analysis.Findings))); err != nil {
				e.failPersistence(r.ID)
				return
			}
			if err := e.store.SetStage(ctx, st.ID, model.Succeeded); err != nil {
				e.failPersistence(r.ID)
				return
			}
			if err := e.store.Event(ctx, r.ID, key, "analysis.clean", "info", map[string]any{"execution": "LIVE", "finding_count": len(analysis.Findings), "critical": analysis.Critical, "high": analysis.High, "medium": analysis.Medium}); err != nil {
				e.failPersistence(r.ID)
				return
			}
			next := ""
			if i+1 < len(stages) {
				next = stages[i+1]
			}
			if checkpointErr := e.saveCursorCheckpoint(ctx, r, featureDir, key, next, configuration.SHA256, "LIVE", loopBudget.Counts()); checkpointErr != nil {
				e.failPersistence(r.ID)
				return
			}
			continue
		}
		if key == "tests" && hasLiveStages {
			if err := e.store.SetStage(ctx, st.ID, model.StageRun); err != nil {
				e.failPersistence(r.ID)
				return
			}
			verdicts, testErr := e.runTestRepairLoop(ctx, r, "tests", decisions, installation, featureDir, loopBudget)
			if persistErr := persistTestVerdicts(ctx, e.store, r.ID, verdicts); persistErr != nil {
				_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
				e.finish(r.ID)
				return
			}
			if testErr != nil {
				action, _ := e.store.RunControl(context.Background(), r.ID)
				if ctx.Err() != nil || action == "cancel" {
					if persistErr := e.cancelRun(r.ID, key, st.ID); persistErr != nil {
						e.failPersistence(r.ID)
					} else {
						e.finish(r.ID)
					}
					return
				}
				if persistErr := e.store.SetStage(context.Background(), st.ID, model.StageFail); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				gateVerdict := testGateVerdict(verdicts)
				if persistErr := e.store.Gate(context.Background(), r.ID, "required_tests", gateVerdict, safeStageError(testErr)); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if persistErr := e.store.Event(context.Background(), r.ID, key, "tests.failed", "error", map[string]any{"reason": safeStageError(testErr), "execution": "LIVE"}); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if persistErr := e.store.SetRunState(context.Background(), r.ID, model.Blocked); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				e.finish(r.ID)
				return
			}
			runGateVerdicts["required_tests"] = "PASS"
			if err := e.store.SetStage(ctx, st.ID, model.Succeeded); err != nil {
				e.failPersistence(r.ID)
				return
			}
			if err := e.store.Event(ctx, r.ID, key, "tests.passed", "info", map[string]any{"execution": "LOCAL", "check_count": len(verdicts)}); err != nil {
				e.failPersistence(r.ID)
				return
			}
			next := ""
			if i+1 < len(stages) {
				next = stages[i+1]
			}
			if checkpointErr := e.saveCursorCheckpoint(ctx, r, featureDir, key, next, configuration.SHA256, "LOCAL", loopBudget.Counts()); checkpointErr != nil {
				e.failPersistence(r.ID)
				return
			}
			continue
		}
		if key == "final_review" && hasLiveStages && d.Provider != "fake" {
			if err := e.store.SetStage(ctx, st.ID, model.StageRun); err != nil {
				e.failPersistence(r.ID)
				return
			}
			err := e.runLiveFinalReviewLoop(ctx, r, decisions, installation, featureDir, runGateVerdicts, loopBudget)
			if err != nil {
				if persistErr := e.failLiveQualityStage(ctx, r.ID, key, st.ID, "final_review", err, ctx); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				e.finish(r.ID)
				return
			}
			runGateVerdicts["final_review"] = "PASS"
			next := ""
			if i+1 < len(stages) {
				next = stages[i+1]
			}
			if err := e.completeLiveQualityStage(ctx, r.ID, st.ID, key, next, configuration.SHA256, "final_review", "read-only final review and any bounded fix/test/convergence cycle completed", loopBudget, featureDir); err != nil {
				_ = e.store.SetRunState(context.Background(), r.ID, model.Failed)
				e.finish(r.ID)
				return
			}
			continue
		}
		if d.Provider != "fake" {
			if err := e.store.SetStage(ctx, st.ID, model.StageRun); err != nil {
				e.failPersistence(r.ID)
				return
			}
			evidence, cancelled, runErr := e.runLiveStageWithControl(ctx, r, key, d, r.Prompt, installation, featureDir)
			if runErr != nil {
				if cancelled {
					if persistErr := e.cancelRun(r.ID, key, st.ID); persistErr != nil {
						e.failPersistence(r.ID)
					} else {
						e.finish(r.ID)
					}
					return
				}
				if persistErr := e.store.SetStage(context.Background(), st.ID, model.StageFail); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if persistErr := e.store.Event(context.Background(), r.ID, key, "stage.failed", "error", map[string]any{"reason": safeStageError(runErr), "execution": "LIVE"}); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				if persistErr := e.store.SetRunState(context.Background(), r.ID, model.Failed); persistErr != nil {
					e.failPersistence(r.ID)
					return
				}
				e.finish(r.ID)
				return
			}
			if err := e.store.SetStage(ctx, st.ID, model.Succeeded); err != nil {
				e.failPersistence(r.ID)
				return
			}
			delete(evidence, "_stage_output")
			evidence["execution"] = "LIVE"
			if err := e.store.Event(ctx, r.ID, key, "stage.completed", "info", evidence); err != nil {
				e.failPersistence(r.ID)
				return
			}
			next := ""
			if i+1 < len(stages) {
				next = stages[i+1]
			}
			if checkpointErr := e.saveCursorCheckpoint(ctx, r, featureDir, key, next, configuration.SHA256, "LIVE", loopBudget.Counts()); checkpointErr != nil {
				e.failPersistence(r.ID)
				return
			}
			continue
		}
		if !e.waitStage(ctx, r.ID, 90*time.Millisecond) {
			if ctx.Err() != nil {
				if err := e.cancelRun(r.ID, key, st.ID); err != nil {
					e.failPersistence(r.ID)
					return
				}
			} else if action, _ := e.store.RunControl(context.Background(), r.ID); action == "cancel" {
				if err := e.cancelRun(r.ID, key, st.ID); err != nil {
					e.failPersistence(r.ID)
					return
				}
			}
			e.finish(r.ID)
			return
		}
		fakeRunner := e.fakeAdapter
		if scenario, ok := e.fakeScenarios[key]; ok {
			fakeRunner = scenario
		}
		fakeEvents, fakeErr := fakeRunner.Execute(ctx)
		for _, event := range fakeEvents {
			if err := e.store.Event(ctx, r.ID, key, "provider.event", "info", map[string]any{"simulated": true, "type": event.Type, "summary": event.Summary}); err != nil {
				e.failPersistence(r.ID)
				return
			}
		}
		if fakeErr != nil {
			if err := e.failSimulatedStage(ctx, r.ID, key, st.ID, fakeErr); err != nil {
				e.failPersistence(r.ID)
			}
			e.finish(r.ID)
			return
		}
		if err := e.store.SetStage(ctx, st.ID, model.Succeeded); err != nil {
			e.failPersistence(r.ID)
			return
		}
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
		if err := e.store.Event(ctx, r.ID, key, typ, "info", map[string]any{"simulated": true, "evidence": "deterministic fake-provider fixture"}); err != nil {
			e.failPersistence(r.ID)
			return
		}
		if hasLiveStages {
			gateKey := map[string]string{"analyze": "analyze_clean", "converge": "converged", "final_review": "final_review"}[key]
			if gateKey != "" {
				runGateVerdicts[gateKey] = "SIMULATED"
				if err := e.store.Gate(ctx, r.ID, gateKey, "SIMULATED", "deterministic fake-provider stage; does not satisfy a live acceptance gate"); err != nil {
					e.failPersistence(r.ID)
					return
				}
			}
		}
		next := ""
		if i+1 < len(stages) {
			next = stages[i+1]
		}
		if checkpointErr := e.saveCursorCheckpoint(ctx, r, featureDir, key, next, configuration.SHA256, "SIMULATED", loopBudget.Counts()); checkpointErr != nil {
			e.failPersistence(r.ID)
			return
		}
	}
	if hasLiveStages {
		for _, gate := range []string{"analyze_clean", "required_tests", "converged", "final_review"} {
			verdict, ok := runGateVerdicts[gate]
			if !ok {
				verdict = "BLOCKED"
			}
			if err := e.store.Gate(ctx, r.ID, gate, verdict, "required live/local evidence for final acceptance"); err != nil {
				e.failPersistence(r.ID)
				return
			}
		}
		if err := e.store.Gate(ctx, r.ID, "simulated_e2e", "NOT_RUN", "simulation-only gate does not apply to live or mixed execution"); err != nil {
			e.failPersistence(r.ID)
			return
		}
		gates, gateErr := e.store.Gates(ctx, r.ID)
		if gateErr != nil {
			e.failPersistence(r.ID)
			return
		}
		acceptance := model.SummarizeAcceptance(acceptanceDecisions(decisions), gates)
		terminal := model.Blocked
		eventType := "run.blocked"
		severity := "warning"
		if acceptance.Verdict == "PASS" {
			terminal = model.Completed
			eventType = "run.completed"
			severity = "info"
		} else if acceptance.Verdict == "FAIL" {
			terminal = model.Failed
			eventType = "run.failed"
		}
		if err := e.store.Event(ctx, r.ID, "", eventType, severity, map[string]any{"execution": acceptance.Execution, "verdict": acceptance.Verdict}); err != nil {
			e.failPersistence(r.ID)
			return
		}
		if err := e.store.SetRunState(ctx, r.ID, terminal); err != nil {
			e.failPersistence(r.ID)
			return
		}
		e.finish(r.ID)
		return
	}
	for _, g := range []string{"analyze_clean", "required_tests", "converged", "final_review", "simulated_e2e"} {
		if err := e.store.Gate(ctx, r.ID, g, "PASS", "simulated deterministic workflow"); err != nil {
			e.failPersistence(r.ID)
			return
		}
	}
	if err := e.store.Event(ctx, r.ID, "", "run.completed", "info", map[string]any{"execution": "SIMULATED", "verdict": "SIMULATED"}); err != nil {
		e.failPersistence(r.ID)
		return
	}
	if err := e.store.SetRunState(ctx, r.ID, model.Completed); err != nil {
		e.failPersistence(r.ID)
		return
	}
	e.finish(r.ID)
}

func (e *Engine) failSimulatedStage(ctx context.Context, runID, key, stageID string, cause error) error {
	reason := safeStageError(cause)
	if err := e.store.SetStage(context.Background(), stageID, model.StageFail); err != nil {
		return err
	}
	if err := e.store.Event(context.Background(), runID, key, "stage.failed", "error", map[string]any{"simulated": true, "reason": reason, "execution": "SIMULATED"}); err != nil {
		return err
	}
	for _, gate := range []string{"analyze_clean", "required_tests", "converged", "final_review", "simulated_e2e"} {
		verdict := "BLOCKED"
		evidence := "simulated workflow stopped before this gate"
		if (key == "analyze" && gate == "analyze_clean") || (key == "tests" && gate == "required_tests") || (key == "converge" && gate == "converged") || (key == "final_review" && gate == "final_review") || (key != "analyze" && key != "tests" && key != "converge" && key != "final_review" && gate == "simulated_e2e") {
			verdict, evidence = "FAIL", reason
		}
		if err := e.store.Gate(context.Background(), runID, gate, verdict, "SIMULATED: "+evidence); err != nil {
			return err
		}
	}
	if err := e.store.SetRunState(context.Background(), runID, model.Failed); err != nil {
		return err
	}
	return nil
}

func (e *Engine) waitForSensitiveApproval(ctx context.Context, runID, stage, stageID string, approval model.Approval, resume <-chan struct{}) (bool, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := e.cancelRun(runID, stage, stageID); err != nil {
				return false, err
			}
			return false, nil
		case <-ticker.C:
			current, err := e.store.Approval(ctx, runID, approval.ID)
			if err != nil {
				return false, err
			}
			switch current.Status {
			case model.ApprovalApproved:
				_ = e.store.ClearRunControl(ctx, runID)
				run, err := e.store.GetRun(ctx, runID)
				if err != nil {
					return false, err
				}
				if run.State == model.Paused {
					if err = e.store.SetRunState(ctx, runID, model.Running); err != nil {
						return false, err
					}
					if err = e.store.Event(ctx, runID, stage, "run.resumed", "info", map[string]any{"stage": stage, "approval_id": approval.ID}); err != nil {
						return false, err
					}
				}
				return true, nil
			case model.ApprovalDenied:
				return false, e.denySensitiveApproval(ctx, runID, stage, stageID, current)
			}
			if action, err := e.store.RunControl(ctx, runID); err != nil {
				return false, err
			} else if action == "cancel" {
				if err = e.cancelRun(runID, stage, stageID); err != nil {
					return false, err
				}
				return false, nil
			}
		case <-resume:
			// Approval resolution is authoritative; this signal only shortens the polling interval.
			current, err := e.store.Approval(ctx, runID, approval.ID)
			if err != nil {
				return false, err
			}
			if current.Status == model.ApprovalApproved {
				_ = e.store.ClearRunControl(ctx, runID)
				run, err := e.store.GetRun(ctx, runID)
				if err != nil {
					return false, err
				}
				if run.State == model.Paused {
					if err = e.store.SetRunState(ctx, runID, model.Running); err != nil {
						return false, err
					}
					if err = e.store.Event(ctx, runID, stage, "run.resumed", "info", map[string]any{"stage": stage, "approval_id": approval.ID}); err != nil {
						return false, err
					}
				}
				return true, nil
			}
			if current.Status == model.ApprovalDenied {
				return false, e.denySensitiveApproval(ctx, runID, stage, stageID, current)
			}
		}
	}
}

func (e *Engine) denySensitiveApproval(ctx context.Context, runID, stage, stageID string, approval model.Approval) error {
	if err := e.store.SetStage(ctx, stageID, model.Skipped); err != nil {
		return err
	}
	if err := e.store.Gate(ctx, runID, "sensitive_action_authorized", "BLOCKED", "workspace modification approval was denied"); err != nil {
		return err
	}
	if err := e.store.Event(ctx, runID, stage, "approval.denied", "warning", map[string]any{"approval_id": approval.ID, "execution": "NOT_RUN"}); err != nil {
		return err
	}
	return e.store.SetRunState(ctx, runID, model.Blocked)
}

func acceptanceDecisions(decisions map[string]config.Decision) []model.RoutingDecision {
	out := make([]model.RoutingDecision, 0, len(decisions))
	for stage, decision := range decisions {
		route := decision.Route
		out = append(out, model.RoutingDecision{Stage: stage, Provider: route.Provider, Profile: route.Profile, Model: route.Model, Effort: route.Effort, Policy: string(decision.Mode), Explanation: decision.Explanation})
	}
	return out
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
	delete(e.resumes, id)
	_ = e.store.ClearRunControl(context.Background(), id)
	if path := e.locks[id]; path != "" {
		_ = e.store.ReleaseWorkspaceLock(context.Background(), path, id)
		delete(e.locks, id)
	}
	if lock := e.processLocks[id]; lock != nil {
		_ = lock.Close()
		delete(e.processLocks, id)
	}
}
