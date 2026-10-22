package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	runConfiguration, err := e.store.RunConfiguration(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	effectiveConfiguration, err := parseEffectiveRunConfiguration(runConfiguration.ConfigJSON)
	if err != nil {
		return nil, err
	}
	if _, err := workspaceguard.Canonical(run.Project, profile.AllowedScopes); err != nil {
		return nil, fmt.Errorf("PROFILE_NOT_ALLOWED: %s: %w", route.Profile, err)
	}
	if err := routing.ValidateRouteCapability(stage, route, *profile); err != nil {
		return nil, err
	}
	approvedExecutable, approvedConfig, err := e.validateProfileReferences(ctx, run.ID, *profile)
	if err != nil {
		return nil, err
	}
	if effectiveConfiguration.SchemaVersion == 1 {
		pinned, ok := effectiveConfiguration.profile(profile.Name)
		if !ok || !profileMatchesReference(*profile, pinned) || approvedExecutable.Path != pinned.ApprovedExecutablePath || approvedExecutable.SHA256 != pinned.ExecutableSHA256 || approvedConfig != pinned.ApprovedConfigDirectory {
			return nil, fmt.Errorf("RUN_CONFIGURATION_PROFILE_DRIFT: %s", profile.Name)
		}
	}
	var before string
	if stage == "analyze" {
		before, err = workspaceFingerprint(installation.ProjectRoot)
		if err != nil {
			return nil, err
		}
	}
	var oldTasks []byte
	var convergenceTaskPath string
	var convergenceBefore string
	var convergencePlan []byte
	if stage == "converge" {
		oldTasks, err = os.ReadFile(filepath.Join(featureDir, "tasks.md"))
		if err != nil {
			return nil, fmt.Errorf("CONVERGENCE_TASKS_UNAVAILABLE: %w", err)
		}
		convergenceTaskPath, err = filepath.Rel(installation.ProjectRoot, filepath.Join(featureDir, "tasks.md"))
		if err != nil || convergenceTaskPath == ".." || strings.HasPrefix(convergenceTaskPath, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("CONVERGENCE_TASKS_PATH_INVALID")
		}
		convergenceTaskPath = filepath.ToSlash(convergenceTaskPath)
		convergenceBefore, err = workspaceFingerprintExcept(installation.ProjectRoot, []string{convergenceTaskPath})
		if err != nil {
			return nil, err
		}
		convergencePlan, err = os.ReadFile(filepath.Join(featureDir, "plan.md"))
		if err != nil {
			return nil, fmt.Errorf("CONVERGENCE_PLAN_UNAVAILABLE: %w", err)
		}
	}
	beforeArtifacts, err := captureStageArtifacts(installation.ProjectRoot, featureDir, stage)
	if err != nil {
		return nil, err
	}
	request := adapters.Request{
		ProjectRoot:      installation.ProjectRoot,
		FeatureDirectory: featureDir,
		Prompt:           prompt,
		Model:            route.Model,
		Effort:           route.Effort,
		Timeout:          time.Duration(effectiveConfiguration.Execution.StageTimeoutSeconds[stage]) * time.Second,
		Profile: adapters.Profile{
			Name: profile.Name, Provider: adapters.ProviderName(profile.Provider),
			Executable: profile.Executable, ConfigDir: approvedConfig,
			AllowedProjects:        append([]string(nil), profile.AllowedScopes...),
			ApprovedExecutablePath: approvedExecutable.Path, ApprovedExecutableSHA256: approvedExecutable.SHA256,
			ApprovedConfigDirectory: approvedConfig,
		},
	}
	option := liveStageOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	if stage == "implement" {
		e.mu.Lock()
		resume := e.resumes[run.ID]
		e.mu.Unlock()
		if err = e.authorizeSensitiveInvocation(ctx, run, stage, phase, installation.ProjectRoot, *profile, resume); err != nil {
			return nil, err
		}
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
	if stage == "analyze" {
		// Check the read-only boundary even when the provider exits with an
		// error or cancellation; a failed invocation may still have written.
		after, snapshotErr := workspaceFingerprint(installation.ProjectRoot)
		if snapshotErr != nil {
			runErr = fmt.Errorf("ANALYZE_MUTATION_CHECK_FAILED: %w", snapshotErr)
			status = "FAILED"
		} else if before != after {
			runErr = fmt.Errorf("ANALYZE_MUTATED_ARTIFACTS")
			status = "FAILED"
		}
	}
	if stage == "converge" {
		// Convergence may append to the existing task ledger only. Compare the
		// rest of the workspace after every invocation, including failures.
		after, snapshotErr := workspaceFingerprintExcept(installation.ProjectRoot, []string{convergenceTaskPath})
		if snapshotErr != nil {
			runErr = fmt.Errorf("CONVERGENCE_MUTATION_CHECK_FAILED: %w", snapshotErr)
			status = "FAILED"
		} else if convergenceBefore != after {
			runErr = fmt.Errorf("CONVERGENCE_CHANGED_NON_LEDGER_ARTIFACTS")
			status = "FAILED"
		}
		currentTasks, readErr := os.ReadFile(filepath.Join(featureDir, "tasks.md"))
		if readErr != nil {
			runErr = fmt.Errorf("CONVERGENCE_TASKS_UNAVAILABLE: %w", readErr)
			status = "FAILED"
		} else if string(oldTasks) != string(currentTasks) {
			if appendErr := speckit.ValidateAppendOnlyWithPlan(string(oldTasks), string(currentTasks), string(convergencePlan)); appendErr != nil {
				runErr = appendErr
				status = "FAILED"
			}
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
			analysis, parseErr := speckit.ParseAnalysisOutput(result.Output)
			if parseErr != nil {
				runErr = parseErr
			} else {
				evidence["analysis_findings"] = len(analysis.Findings)
				evidence["analysis_critical"] = analysis.Critical
				evidence["analysis_high"] = analysis.High
				evidence["analysis_medium"] = analysis.Medium
			}
		case "converge":
			currentTasks, readErr := os.ReadFile(filepath.Join(featureDir, "tasks.md"))
			if readErr != nil {
				runErr = fmt.Errorf("CONVERGENCE_TASKS_UNAVAILABLE: %w", readErr)
			} else if convergence, parseErr := speckit.ParseConvergenceOutputWithPlan(result.Output, string(convergencePlan)); parseErr != nil {
				runErr = parseErr
			} else if convergence.Converged && string(oldTasks) != string(currentTasks) {
				runErr = fmt.Errorf("CONVERGENCE_CHANGED_TASKS_WITHOUT_APPEND_REPORT")
			} else if !convergence.Converged {
				if validateErr := speckit.ValidateAppendOnlyWithPlan(string(oldTasks), string(currentTasks), string(convergencePlan)); validateErr != nil {
					runErr = validateErr
				} else if appended, appendErr := speckit.AppendedConvergenceTasksWithPlan(string(oldTasks), string(currentTasks), string(convergencePlan)); appendErr != nil {
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
		case "clarify":
			registered, runErr = changedStageArtifacts(installation.ProjectRoot, featureDir, stage, beforeArtifacts)
			if runErr == nil && len(registered) == 0 {
				var reason string
				reason, runErr = requireExplicitNoop(result.Output, "SPECTIDE_NO_CLARIFICATION")
				if runErr == nil {
					runErr = validateExistingSpecification(featureDir)
				}
				if runErr == nil {
					evidence["no_op_reason"] = reason
				}
			} else if runErr == nil {
				if _, markerErr := requireExplicitNoop(result.Output, "SPECTIDE_NO_CLARIFICATION"); markerErr == nil {
					runErr = fmt.Errorf("STAGE_NO_OP_CONTRADICTS_ARTIFACT_CHANGE")
				}
			}
		case "implement":
			registered, runErr = changedStageArtifacts(installation.ProjectRoot, featureDir, stage, beforeArtifacts)
			if runErr == nil && len(registered) == 0 {
				var reason string
				reason, runErr = requireExplicitNoop(result.Output, "SPECTIDE_NO_WORK")
				if runErr == nil {
					runErr = validateExistingImplementation(installation.ProjectRoot)
				}
				if runErr == nil {
					evidence["no_op_reason"] = reason
				}
			} else if runErr == nil {
				if _, markerErr := requireExplicitNoop(result.Output, "SPECTIDE_NO_WORK"); markerErr == nil {
					runErr = fmt.Errorf("STAGE_NO_OP_CONTRADICTS_ARTIFACT_CHANGE")
				}
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
	if runErr == nil && len(registered) > 0 {
		runErr = validateChangedOwnerArtifact(stage, featureDir, registered)
	}
	if runErr == nil && stage == "converge" {
		registered, runErr = changedStageArtifacts(installation.ProjectRoot, featureDir, stage, beforeArtifacts)
	}
	if runErr == nil && stage == "specify" {
		runErr = speckit.ValidateUniqueFeatureDirectory(installation.ProjectRoot, featureDir)
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
	evidence["_stage_output"] = result.Output
	return evidence, nil
}

func (e *Engine) authorizeSensitiveInvocation(ctx context.Context, run model.Run, stage, phase, projectRoot string, profile model.Profile, resume <-chan struct{}) error {
	stages, err := e.store.Stages(ctx, run.ID)
	if err != nil {
		return err
	}
	activeStage := model.Stage{}
	for _, candidate := range stages {
		if candidate.State == model.StageRun && (activeStage.ID == "" || candidate.Position > activeStage.Position || candidate.Attempt > activeStage.Attempt) {
			activeStage = candidate
		}
	}
	if activeStage.ID == "" {
		return fmt.Errorf("SENSITIVE_WRITE_STAGE_REQUIRED")
	}
	workspaceScope := workspaceIdentityFingerprint(projectRoot)
	action := fmt.Sprintf("Write workspace %s during %s/%s via %s/%s", workspaceScope, stage, phase, profile.Provider, profile.Name)
	approval, err := e.store.CreateWorkflowApprovalAndPause(ctx, run.ID, activeStage.Key, action)
	if err != nil {
		return err
	}
	if approval.Status == model.ApprovalPending {
		approved, waitErr := e.waitForSensitiveApproval(ctx, run.ID, activeStage.Key, activeStage.ID, approval, resume)
		if waitErr != nil {
			return waitErr
		}
		if !approved {
			if err = e.store.Event(context.Background(), run.ID, activeStage.Key, "approval.denied", "warning", map[string]any{"approval_id": approval.ID, "stage": stage, "phase": phase, "execution": "NOT_RUN"}); err != nil {
				return err
			}
			if err = e.store.Gate(context.Background(), run.ID, "sensitive_action_authorized", "BLOCKED", "workspace modification approval was denied for this invocation"); err != nil {
				return err
			}
			return fmt.Errorf("SENSITIVE_ACTION_NOT_APPROVED")
		}
	} else if approval.Status == model.ApprovalDenied {
		if err = e.store.Event(context.Background(), run.ID, activeStage.Key, "approval.denied", "warning", map[string]any{"approval_id": approval.ID, "stage": stage, "phase": phase, "execution": "NOT_RUN"}); err != nil {
			return err
		}
		if err = e.store.Gate(context.Background(), run.ID, "sensitive_action_authorized", "BLOCKED", "workspace modification approval was denied for this invocation"); err != nil {
			return err
		}
		return fmt.Errorf("SENSITIVE_ACTION_DENIED")
	} else if approval.Status != model.ApprovalApproved {
		return fmt.Errorf("SENSITIVE_ACTION_STATUS_INVALID")
	}
	return e.store.Event(ctx, run.ID, activeStage.Key, "approval.write_authorized", "info", map[string]any{"approval_id": approval.ID, "stage": stage, "phase": phase, "workspace_scope": workspaceScope, "provider": profile.Provider, "profile": profile.Name, "execution": "LIVE"})
}

func (e *Engine) validateProfileReferences(ctx context.Context, runID string, profile model.Profile) (security.ExecutableReference, string, error) {
	capability := profile.Capability
	if capability == nil || !capability.ExecutableFound || capability.ApprovedExecutablePath == "" || capability.ExecutableSHA256 == "" {
		return security.ExecutableReference{}, "", fmt.Errorf("PROFILE_CAPABILITY_REFERENCE_NOT_PROBED")
	}
	executable, err := security.ResolveExecutableReference(profile.Executable)
	if err != nil {
		return security.ExecutableReference{}, "", err
	}
	configDirectory, err := security.ResolveConfigDirectory(profile.ConfigDirectory)
	if err != nil {
		return security.ExecutableReference{}, "", err
	}
	if executable.Path != capability.ApprovedExecutablePath || executable.SHA256 != capability.ExecutableSHA256 || configDirectory != capability.ApprovedConfigDirectory {
		if err = e.store.InvalidateProfileCapabilityForRun(context.Background(), profile.Name, runID); err != nil {
			return security.ExecutableReference{}, "", err
		}
		return security.ExecutableReference{}, "", fmt.Errorf("PROFILE_CAPABILITY_REFERENCE_DRIFT")
	}
	return executable, configDirectory, nil
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
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == ".next" || entry.Name() == "target" || entry.Name() == ".venv") {
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
	return workspaceFingerprintExcept(root, nil)
}

func workspaceFingerprintExcept(root string, excludedPaths []string) (string, error) {
	excluded := make(map[string]struct{}, len(excludedPaths))
	for _, path := range excludedPaths {
		excluded[filepath.ToSlash(filepath.Clean(path))] = struct{}{}
	}
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
		if _, skip := excluded[path]; skip {
			continue
		}
		_, _ = fmt.Fprintf(h, "%s:%s\n", path, files[path].SHA256)
	}
	// Include directory and symlink entries so empty-directory additions,
	// removals, and link retargeting cannot evade read-only checks.
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	err = filepath.WalkDir(canonicalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(canonicalRoot, path)
		if relErr != nil {
			return relErr
		}
		if path != canonicalRoot && entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == ".next" || entry.Name() == "target" || entry.Name() == ".venv") {
			return filepath.SkipDir
		}
		if path == canonicalRoot {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, skip := excluded[rel]; skip {
			return nil
		}
		if entry.IsDir() {
			_, _ = fmt.Fprintf(h, "dir:%s\n", rel)
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			_, _ = fmt.Fprintf(h, "symlink:%s:%s\n", rel, target)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	gitState, err := workspaceGitState(canonicalRoot, excluded)
	if err != nil {
		return "", err
	}
	_, _ = fmt.Fprintf(h, "git:%s\n", gitState)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func workspaceGitState(root string, excluded map[string]struct{}) (string, error) {
	if _, err := os.Lstat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		return "NO_GIT_METADATA", nil
	} else if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "-C", root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("WORKSPACE_GIT_STATE_UNAVAILABLE: %w", err)
	}
	head, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("WORKSPACE_GIT_HEAD_UNAVAILABLE: %w", err)
	}
	var status strings.Builder
	records := strings.Split(string(output), "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		path := filepath.ToSlash(record[3:])
		_, skipPath := excluded[path]
		if (record[0] == 'R' || record[0] == 'C' || record[1] == 'R' || record[1] == 'C') && i+1 < len(records) {
			source := filepath.ToSlash(records[i+1])
			_, skipSource := excluded[source]
			if skipPath && skipSource {
				i++
				continue
			}
			if skipPath || skipSource {
				i++
				status.WriteString("RENAME_WITH_NON_LEDGER_PATH\n")
				continue
			}
			status.WriteString(record)
			status.WriteByte(0)
			status.WriteString(records[i+1])
			status.WriteByte(0)
			i++
			continue
		}
		if !skipPath {
			status.WriteString(record)
			status.WriteByte(0)
		}
	}
	return hashString(string(head) + "\n" + status.String()), nil
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
