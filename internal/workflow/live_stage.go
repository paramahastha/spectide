package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/adapters"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/routing"
	"github.com/spectide/spectide/internal/security"
	"github.com/spectide/spectide/internal/speckit"
	workspaceguard "github.com/spectide/spectide/internal/workspace"
)

func (e *Engine) runLiveStageWithControl(ctx context.Context, run model.Run, stage string, route config.Route, prompt string, installation speckit.Installation, featureDir string) (map[string]any, bool, error) {
	return e.runLiveStageWithControlPhase(ctx, run, stage, stage, route, prompt, installation, featureDir)
}

func (e *Engine) runLiveStageWithControlPhase(ctx context.Context, run model.Run, stage, phase string, route config.Route, prompt string, installation speckit.Installation, featureDir string) (map[string]any, bool, error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-child.Done():
				return
			case <-ticker.C:
				action, err := e.store.RunControl(context.Background(), run.ID)
				if err == nil && action == "cancel" {
					cancel()
					return
				}
			}
		}
	}()
	evidence, err := e.runLiveStagePromptPhase(child, run, stage, phase, route, prompt, installation, featureDir)
	close(stop)
	<-done
	if err != nil {
		action, _ := e.store.RunControl(context.Background(), run.ID)
		cancelled := ctx.Err() != nil || action == "cancel"
		if cancelled {
			return nil, true, err
		}
		return nil, false, err
	}
	return evidence, false, nil
}

func (e *Engine) runLiveStage(ctx context.Context, run model.Run, stage string, route config.Route, installation speckit.Installation, featureDir string) (map[string]any, error) {
	return e.runLiveStagePrompt(ctx, run, stage, route, run.Prompt, installation, featureDir)
}

func (e *Engine) runLiveStagePrompt(ctx context.Context, run model.Run, stage string, route config.Route, prompt string, installation speckit.Installation, featureDir string) (map[string]any, error) {
	return e.runLiveStagePromptPhase(ctx, run, stage, stage, route, prompt, installation, featureDir)
}

func (e *Engine) runLiveStagePromptPhase(ctx context.Context, run model.Run, stage, phase string, route config.Route, prompt string, installation speckit.Installation, featureDir string) (map[string]any, error) {
	adapter := e.adapters[adapters.ProviderName(route.Provider)]
	if adapter == nil {
		return nil, fmt.Errorf("PROVIDER_ADAPTER_UNAVAILABLE: %s", route.Provider)
	}
	profiles, err := e.store.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	var profile *model.Profile
	for i := range profiles {
		if profiles[i].Name == route.Profile {
			profile = &profiles[i]
			break
		}
	}
	if profile == nil {
		return nil, fmt.Errorf("PROFILE_NOT_FOUND: %s", route.Profile)
	}
	if _, err := workspaceguard.Canonical(run.Project, profile.AllowedScopes); err != nil {
		return nil, fmt.Errorf("PROFILE_NOT_ALLOWED: %s: %w", route.Profile, err)
	}
	if err := routing.ValidateRouteCapability(stage, route, *profile); err != nil {
		return nil, err
	}
	var before string
	if stage == "analyze" {
		before, err = fingerprintArtifacts(featureDir)
		if err != nil {
			return nil, err
		}
	}
	var oldTasks []byte
	if stage == "converge" {
		oldTasks, err = os.ReadFile(filepath.Join(featureDir, "tasks.md"))
		if err != nil {
			return nil, fmt.Errorf("CONVERGENCE_TASKS_UNAVAILABLE: %w", err)
		}
	}
	request := adapters.Request{
		ProjectRoot: installation.ProjectRoot,
		Prompt:      prompt,
		Model:       route.Model,
		Effort:      route.Effort,
		Profile: adapters.Profile{
			Name: profile.Name, Provider: adapters.ProviderName(profile.Provider),
			Executable: profile.Executable, ConfigDir: profile.ConfigDirectory,
			AllowedProjects: append([]string(nil), profile.AllowedScopes...),
		},
	}
	e.mu.Lock()
	if processLock := e.processLocks[run.ID]; processLock != nil && processLock.File() != nil {
		request.InheritedFiles = []*os.File{processLock.File()}
	}
	e.mu.Unlock()
	attempt, err := e.store.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: run.ID, Stage: stage, Phase: phase, Provider: route.Provider, Profile: route.Profile, Model: route.Model, Effort: route.Effort})
	if err != nil {
		return nil, err
	}
	result, runErr := speckit.ExecuteStage(ctx, installation, adapter, request, stage, prompt)
	status := "SUCCEEDED"
	if runErr != nil {
		status = "FAILED"
		if ctx.Err() != nil {
			status = "CANCELLED"
		}
	}
	stdoutHash, stderrHash := hashString(result.Process.Stdout), hashString(result.Process.Stderr)
	eventTypes := make([]string, 0, len(result.Events))
	for _, event := range result.Events {
		eventTypes = append(eventTypes, safeProviderEventType(event.Type))
		if err := e.store.Event(ctx, run.ID, stage, "provider."+safeProviderEventType(event.Type), "info", map[string]any{"summary": security.Redact(event.Summary), "execution": "LIVE"}); err != nil {
			runErr = err
			status = "FAILED"
			break
		}
	}
	attempt.ProcessID = result.Process.ProcessID
	attempt.DurationMillis = result.Process.Duration.Milliseconds()
	attempt.ExitCode = result.Process.ExitCode
	finishedAt := result.Process.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	attempt.FinishedAt = &finishedAt
	attempt.Result = status
	attempt.StdoutSHA256 = stdoutHash
	attempt.StderrSHA256 = stderrHash
	if runErr != nil {
		attempt.Error = safeStageError(runErr)
	}
	evidenceJSON, _ := json.Marshal(map[string]any{"event_types": eventTypes, "event_count": len(result.Events), "stdout_sha256": stdoutHash, "stderr_sha256": stderrHash})
	attempt.EvidenceJSON = string(evidenceJSON)
	if err = e.store.CompleteExecutionAttempt(context.Background(), attempt); err != nil {
		return nil, err
	}
	if runErr != nil {
		return nil, runErr
	}
	evidence := map[string]any{"provider": route.Provider, "profile": route.Profile, "event_count": len(result.Events), "duration_ms": result.Process.Duration.Milliseconds()}
	switch stage {
	case "analyze":
		after, hashErr := fingerprintArtifacts(featureDir)
		if hashErr != nil {
			return nil, hashErr
		}
		if before != after {
			return nil, fmt.Errorf("ANALYZE_MUTATED_ARTIFACTS")
		}
		analysis, parseErr := speckit.ParseAnalysisOutput(result.Process.Stdout)
		if parseErr != nil {
			return nil, parseErr
		}
		evidence["analysis_findings"] = len(analysis.Findings)
		evidence["analysis_critical"] = analysis.Critical
		evidence["analysis_high"] = analysis.High
		evidence["analysis_medium"] = analysis.Medium
	case "converge":
		currentTasks, readErr := os.ReadFile(filepath.Join(featureDir, "tasks.md"))
		if readErr != nil {
			return nil, fmt.Errorf("CONVERGENCE_TASKS_UNAVAILABLE: %w", readErr)
		}
		convergence, parseErr := speckit.ParseConvergenceOutput(result.Process.Stdout)
		if parseErr != nil {
			return nil, parseErr
		}
		if convergence.Converged {
			if string(oldTasks) != string(currentTasks) {
				return nil, fmt.Errorf("CONVERGENCE_CHANGED_TASKS_WITHOUT_APPEND_REPORT")
			}
		} else if err := speckit.ValidateAppendOnly(string(oldTasks), string(currentTasks)); err != nil {
			return nil, err
		}
		evidence["converged"] = convergence.Converged
		evidence["appended_task_count"] = len(convergence.Appended)
	}
	evidence["_stage_output"] = result.Process.Stdout
	return evidence, nil
}

func hashString(value string) string {
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func fingerprintArtifacts(featureDir string) (string, error) {
	state, err := speckit.InspectArtifacts(featureDir)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, name := range []string{"spec", "plan", "tasks"} {
		path := map[string]string{"spec": state.SpecPath, "plan": state.PlanPath, "tasks": state.TasksPath}[name]
		if !state.Present[name] {
			_, _ = fmt.Fprintf(h, "%s:absent\n", name)
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "%s:%x\n", name, sha256.Sum256(content))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func safeProviderEventType(value string) string {
	if value == "" || len(value) > 96 {
		return "event"
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && !strings.ContainsRune("_.-", r) {
			return "event"
		}
	}
	return value
}

func safeStageError(err error) string {
	return security.Redact(err.Error())
}
