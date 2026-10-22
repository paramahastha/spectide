package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	return e.runLiveStageWithControlOptions(ctx, run, stage, phase, route, prompt, installation, featureDir)
}

type liveStageOptions struct {
	templateStage string
	ownerPath     string
}

func (e *Engine) runLiveStageWithControlOptions(ctx context.Context, run model.Run, stage, phase string, route config.Route, prompt string, installation speckit.Installation, featureDir string, options ...liveStageOptions) (map[string]any, bool, error) {
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
	evidence, err := e.runLiveStagePromptPhase(child, run, stage, phase, route, prompt, installation, featureDir, options...)
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

func (e *Engine) runLiveStagePromptPhase(ctx context.Context, run model.Run, stage, phase string, route config.Route, prompt string, installation speckit.Installation, featureDir string, options ...liveStageOptions) (map[string]any, error) {
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
		before, err = workspaceFingerprint(installation.ProjectRoot)
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
	beforeArtifacts, err := captureStageArtifacts(installation.ProjectRoot, featureDir, stage)
	if err != nil {
		return nil, err
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
	option := liveStageOptions{}
	if len(options) > 0 {
		option = options[0]
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
	templateStage := option.templateStage
	if templateStage == "" {
		templateStage = stage
	}
	result, runErr := speckit.ExecuteStageWithTemplate(ctx, installation, adapter, request, stage, templateStage, prompt)
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
	evidence := map[string]any{"provider": route.Provider, "profile": route.Profile, "event_count": len(result.Events), "duration_ms": result.Process.Duration.Milliseconds()}
	var registered []model.Artifact
	if runErr == nil {
		switch stage {
		case "analyze":
			after, hashErr := workspaceFingerprint(installation.ProjectRoot)
			if hashErr != nil {
				runErr = hashErr
			} else if before != after {
				runErr = fmt.Errorf("ANALYZE_MUTATED_ARTIFACTS")
			} else {
				analysis, parseErr := speckit.ParseAnalysisOutput(result.Process.Stdout)
				if parseErr != nil {
					runErr = parseErr
				} else {
					evidence["analysis_findings"] = len(analysis.Findings)
					evidence["analysis_critical"] = analysis.Critical
					evidence["analysis_high"] = analysis.High
					evidence["analysis_medium"] = analysis.Medium
				}
			}
		case "converge":
			currentTasks, readErr := os.ReadFile(filepath.Join(featureDir, "tasks.md"))
			if readErr != nil {
				runErr = fmt.Errorf("CONVERGENCE_TASKS_UNAVAILABLE: %w", readErr)
			} else if convergence, parseErr := speckit.ParseConvergenceOutput(result.Process.Stdout); parseErr != nil {
				runErr = parseErr
			} else if convergence.Converged && string(oldTasks) != string(currentTasks) {
				runErr = fmt.Errorf("CONVERGENCE_CHANGED_TASKS_WITHOUT_APPEND_REPORT")
			} else if !convergence.Converged {
				if validateErr := speckit.ValidateAppendOnly(string(oldTasks), string(currentTasks)); validateErr != nil {
					runErr = validateErr
				} else if appended, appendErr := speckit.AppendedConvergenceTasks(string(oldTasks), string(currentTasks)); appendErr != nil {
					runErr = appendErr
				} else if !equalStrings(convergence.Appended, appended) {
					runErr = fmt.Errorf("CONVERGENCE_REPORT_TASKS_MISMATCH")
				} else {
					evidence["converged"] = false
					evidence["appended_task_count"] = len(appended)
				}
			} else {
				evidence["converged"] = true
				evidence["appended_task_count"] = 0
			}
		default:
			var inspectErr error
			registered, inspectErr = changedStageArtifacts(installation.ProjectRoot, featureDir, stage, beforeArtifacts)
			if inspectErr != nil {
				runErr = inspectErr
			} else if len(registered) == 0 {
				runErr = fmt.Errorf("STAGE_EXPECTED_ARTIFACT_MISSING_OR_UNCHANGED: %s", stage)
			}
		}
	}
	if runErr == nil && stage == "converge" {
		registered, runErr = changedStageArtifacts(installation.ProjectRoot, featureDir, stage, beforeArtifacts)
	}
	if runErr == nil {
		if option.ownerPath != "" {
			if len(registered) != 1 || filepath.ToSlash(registered[0].Path) != filepath.ToSlash(option.ownerPath) {
				runErr = fmt.Errorf("ANALYZE_REMEDIATION_CHANGED_NON_OWNER_ARTIFACT")
			}
		}
	}
	if runErr == nil {
		for i := range registered {
			registered[i].RunID = run.ID
			registered[i].Stage = stage
			registered[i], runErr = e.store.SaveArtifact(ctx, registered[i])
			if runErr != nil {
				break
			}
		}
	}
	if runErr == nil {
		ids := make([]string, 0, len(registered))
		for _, artifact := range registered {
			ids = append(ids, artifact.ID)
		}
		if len(ids) > 0 {
			evidence["artifact_ids"] = ids
		}
	}
	if runErr != nil {
		status = "FAILED"
		if ctx.Err() != nil {
			status = "CANCELLED"
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
	for _, artifact := range registered {
		attempt.ArtifactIDs = append(attempt.ArtifactIDs, artifact.ID)
	}
	attempt.StdoutSHA256 = stdoutHash
	attempt.StderrSHA256 = stderrHash
	if runErr != nil {
		attempt.Error = safeStageError(runErr)
	}
	evidenceJSON, _ := json.Marshal(map[string]any{"event_types": eventTypes, "event_count": len(result.Events), "stdout_sha256": stdoutHash, "stderr_sha256": stderrHash, "artifact_ids": attempt.ArtifactIDs})
	attempt.EvidenceJSON = string(evidenceJSON)
	if err = e.store.CompleteExecutionAttempt(context.Background(), attempt); err != nil {
		return nil, err
	}
	if runErr != nil {
		return nil, runErr
	}
	evidence["_stage_output"] = result.Process.Stdout
	return evidence, nil
}

type stageFile struct {
	Path   string
	SHA256 string
	Kind   string
}

func captureStageArtifacts(projectRoot, featureDir, stage string) (map[string]stageFile, error) {
	paths := map[string]string{}
	switch stage {
	case "constitution":
		paths[".specify/memory/constitution.md"] = filepath.Join(projectRoot, ".specify", "memory", "constitution.md")
	case "specify", "clarify":
		paths[filepath.Join("specs", filepath.Base(featureDir), "spec.md")] = filepath.Join(featureDir, "spec.md")
	case "plan":
		paths[filepath.Join("specs", filepath.Base(featureDir), "plan.md")] = filepath.Join(featureDir, "plan.md")
	case "checklist":
		paths[filepath.Join("specs", filepath.Base(featureDir), "checklists", "requirements.md")] = filepath.Join(featureDir, "checklists", "requirements.md")
	case "tasks", "converge":
		paths[filepath.Join("specs", filepath.Base(featureDir), "tasks.md")] = filepath.Join(featureDir, "tasks.md")
	case "implement":
		return workspaceFiles(projectRoot)
	default:
		return map[string]stageFile{}, nil
	}
	files := make(map[string]stageFile, len(paths))
	for relative, absolute := range paths {
		file, err := inspectStageFile(projectRoot, relative, absolute)
		if err != nil {
			return nil, err
		}
		if file.SHA256 != "" {
			files[relative] = file
		}
	}
	return files, nil
}

func inspectStageFile(projectRoot, relative, absolute string) (stageFile, error) {
	info, err := os.Lstat(absolute)
	if os.IsNotExist(err) {
		return stageFile{Path: relative, Kind: "markdown"}, nil
	}
	if err != nil {
		return stageFile{}, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return stageFile{}, err
	}
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return stageFile{}, err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return stageFile{}, fmt.Errorf("STAGE_ARTIFACT_PATH_ESCAPE: %s", relative)
	}
	if !info.Mode().IsRegular() {
		return stageFile{}, fmt.Errorf("STAGE_ARTIFACT_NOT_REGULAR: %s", relative)
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return stageFile{}, err
	}
	return stageFile{Path: filepath.ToSlash(relative), SHA256: hashString(string(content)), Kind: artifactKind(relative)}, nil
}

func workspaceFiles(root string) (map[string]stageFile, error) {
	files := map[string]stageFile{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == ".next" || entry.Name() == "vendor" || entry.Name() == "dist" || entry.Name() == "build" || entry.Name() == "target" || entry.Name() == ".venv") {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			canonicalRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				return err
			}
			targetRel, err := filepath.Rel(canonicalRoot, resolved)
			if err != nil || targetRel == ".." || strings.HasPrefix(targetRel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("STAGE_ARTIFACT_PATH_ESCAPE: %s", rel)
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if len(files) >= 20000 {
			return fmt.Errorf("STAGE_ARTIFACT_SCAN_LIMIT")
		}
		file, err := inspectStageFile(root, rel, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = file
		return nil
	})
	return files, err
}

func changedStageArtifacts(projectRoot, featureDir, stage string, before map[string]stageFile) ([]model.Artifact, error) {
	after, err := captureStageArtifacts(projectRoot, featureDir, stage)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(after))
	for path := range after {
		if after[path].SHA256 != "" && after[path].SHA256 != before[path].SHA256 {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	artifacts := make([]model.Artifact, 0, len(paths))
	for _, path := range paths {
		file := after[path]
		artifacts = append(artifacts, model.Artifact{Path: path, Kind: file.Kind, SHA256: file.SHA256})
	}
	return artifacts, nil
}

func artifactKind(path string) string {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if ext == "" {
		return "file"
	}
	return ext
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func workspaceFingerprint(root string) (string, error) {
	files, err := workspaceFiles(root)
	if err != nil {
		return "", err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		_, _ = fmt.Fprintf(h, "%s:%s\n", path, files[path].SHA256)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashString(value string) string {
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
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
