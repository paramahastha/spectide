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
	"reflect"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/speckit"
)

type cursorCheckpointState struct {
	CheckpointID       string            `json:"checkpoint_id"`
	CompletedStage     string            `json:"completed_stage"`
	NextStage          string            `json:"next_stage"`
	ConfigurationSHA   string            `json:"configuration_sha256"`
	Execution          string            `json:"execution"`
	CursorOnly         bool              `json:"cursor_only"`
	WorkspacePath      string            `json:"workspace_path"`
	WorkspaceIdentity  string            `json:"workspace_identity"`
	GitHead            string            `json:"git_head"`
	GitIndex           string            `json:"git_index"`
	ArtifactPathHashes map[string]string `json:"artifact_path_hashes"`
	ManagedPathHashes  map[string]string `json:"managed_path_hashes"`
	LoopCounts         LoopLimits        `json:"loop_counts"`
}

type storedRunDecision struct {
	Stage    string          `json:"stage"`
	Decision config.Decision `json:"decision"`
}

func parseStoredDecisions(encoded string) (map[string]config.Decision, error) {
	var stored []storedRunDecision
	if err := json.Unmarshal([]byte(encoded), &stored); err != nil {
		return nil, fmt.Errorf("WORKSPACE_DRIFT: immutable run configuration is invalid")
	}
	decisions := make(map[string]config.Decision, len(stored))
	for _, item := range stored {
		if indexOfStage(item.Stage) < 0 || item.Decision.Route.Provider == "" {
			return nil, fmt.Errorf("WORKSPACE_DRIFT: immutable run configuration has invalid stage routing")
		}
		decisions[item.Stage] = item.Decision
	}
	if len(decisions) != len(stages) {
		return nil, fmt.Errorf("WORKSPACE_DRIFT: immutable run configuration is incomplete")
	}
	return decisions, nil
}

func (e *Engine) saveCursorCheckpoint(ctx context.Context, run model.Run, featureDir, stage, next, configHash, execution string, counts LoopLimits) error {
	artifacts, err := e.store.Artifacts(ctx, run.ID)
	if err != nil {
		return err
	}
	artifactHashes, managedHashes, gitHead, gitIndex, err := workspaceCheckpointHashes(run.Project, featureDir, artifacts)
	if err != nil {
		return err
	}
	checkpoint := model.Checkpoint{ID: uuid.NewString(), RunID: run.ID, Stage: stage, WorkspaceFingerprint: workspaceIdentityFingerprint(run.Project)}
	completed := stage
	if stage == "starting" {
		completed = ""
	}
	state := cursorCheckpointState{
		CheckpointID: checkpoint.ID, CompletedStage: completed, NextStage: next, ConfigurationSHA: configHash,
		Execution: execution, CursorOnly: true, WorkspacePath: filepath.Clean(run.Project),
		WorkspaceIdentity: checkpoint.WorkspaceFingerprint, GitHead: gitHead, GitIndex: gitIndex,
		ArtifactPathHashes: artifactHashes, ManagedPathHashes: managedHashes, LoopCounts: counts,
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	checkpoint.StateJSON = string(encoded)
	_, err = e.store.SaveCheckpoint(ctx, checkpoint)
	return err
}

func workspaceCheckpointHashes(root, featureDir string, artifacts []model.Artifact) (map[string]string, map[string]string, string, string, error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, "", "", err
	}
	canonical = filepath.Clean(canonical)
	artifactHashes := map[string]string{}
	for _, artifact := range artifacts {
		if err = addWorkspaceFileHash(canonical, artifact.Path, artifactHashes); err != nil {
			return nil, nil, "", "", err
		}
		if artifactHashes[filepath.ToSlash(filepath.Clean(artifact.Path))] != artifact.SHA256 {
			return nil, nil, "", "", fmt.Errorf("WORKSPACE_DRIFT: registered artifact hash does not match: %s", artifact.Path)
		}
	}
	if featureDir != "" {
		state, inspectErr := speckit.InspectArtifacts(featureDir)
		if inspectErr != nil {
			return nil, nil, "", "", inspectErr
		}
		for name, path := range map[string]string{"spec.md": state.SpecPath, "plan.md": state.PlanPath, "tasks.md": state.TasksPath} {
			if !state.Present[strings.TrimSuffix(name, ".md")] {
				continue
			}
			rel, relErr := filepath.Rel(canonical, path)
			if relErr != nil {
				return nil, nil, "", "", relErr
			}
			if err = addWorkspaceFileHash(canonical, filepath.ToSlash(rel), artifactHashes); err != nil {
				return nil, nil, "", "", err
			}
		}
	}
	managed := map[string]string{}
	head, headErr := gitOutput(canonical, "rev-parse", "--verify", "HEAD")
	if headErr != nil {
		return artifactHashes, managed, "", "", nil
	}
	index, indexErr := gitOutput(canonical, "write-tree")
	pathsOutput, pathsErr := gitOutput(canonical, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if indexErr != nil || pathsErr != nil {
		return artifactHashes, managed, "", "", nil
	}
	paths := strings.Split(pathsOutput, "\x00")
	for _, path := range paths {
		if path == "" {
			continue
		}
		rel := filepath.Clean(filepath.FromSlash(path))
		if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, nil, "", "", fmt.Errorf("WORKSPACE_DRIFT: managed path escapes workspace")
		}
		absolute := filepath.Join(canonical, rel)
		info, statErr := os.Lstat(absolute)
		if os.IsNotExist(statErr) {
			managed[filepath.ToSlash(rel)] = "MISSING"
			continue
		}
		if statErr != nil {
			return nil, nil, "", "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, linkErr := os.Readlink(absolute)
			if linkErr != nil {
				return nil, nil, "", "", linkErr
			}
			digest := sha256.Sum256([]byte("symlink:" + target))
			managed[filepath.ToSlash(rel)] = hex.EncodeToString(digest[:])
			continue
		}
		if !info.Mode().IsRegular() {
			managed[filepath.ToSlash(rel)] = "NON_REGULAR"
			continue
		}
		content, readErr := os.ReadFile(absolute)
		if readErr != nil {
			return nil, nil, "", "", readErr
		}
		digest := sha256.Sum256(content)
		managed[filepath.ToSlash(rel)] = hex.EncodeToString(digest[:])
	}
	return artifactHashes, managed, strings.TrimSpace(head), strings.TrimSpace(index), nil
}

func addWorkspaceFileHash(root, relative string, hashes map[string]string) error {
	rel := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("WORKSPACE_DRIFT: artifact path escapes workspace")
	}
	absolute := filepath.Join(root, rel)
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			hashes[filepath.ToSlash(rel)] = "MISSING"
			return nil
		}
		return err
	}
	inside, err := filepath.Rel(root, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return fmt.Errorf("WORKSPACE_DRIFT: artifact path escapes workspace")
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(content)
	hashes[filepath.ToSlash(rel)] = hex.EncodeToString(digest[:])
	return nil
}

func gitOutput(root string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", root}, args...)
	output, err := exec.Command("git", commandArgs...).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (e *Engine) latestCursorCheckpoint(ctx context.Context, runID string) (model.Checkpoint, cursorCheckpointState, error) {
	checkpoints, err := e.store.Checkpoints(ctx, runID)
	if err != nil {
		return model.Checkpoint{}, cursorCheckpointState{}, err
	}
	if len(checkpoints) == 0 {
		return model.Checkpoint{}, cursorCheckpointState{}, fmt.Errorf("WORKSPACE_DRIFT: no verified checkpoint exists")
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		if checkpoints[i].CreatedAt.Equal(checkpoints[j].CreatedAt) {
			return checkpoints[i].ID > checkpoints[j].ID
		}
		return checkpoints[i].CreatedAt.After(checkpoints[j].CreatedAt)
	})
	checkpoint := checkpoints[0]
	var state cursorCheckpointState
	if err = json.Unmarshal([]byte(checkpoint.StateJSON), &state); err != nil {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: checkpoint state is invalid")
	}
	if !state.CursorOnly || state.CheckpointID != checkpoint.ID || checkpoint.WorkspaceFingerprint != state.WorkspaceIdentity {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: checkpoint identity mismatch")
	}
	completed := checkpoint.Stage
	if checkpoint.Stage == "starting" {
		completed = ""
	}
	completedIndex := indexOfStage(checkpoint.Stage)
	if checkpoint.Stage != "starting" && completedIndex < 0 {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: checkpoint stage identity is invalid")
	}
	expectedNext := ""
	if checkpoint.Stage == "starting" {
		expectedNext = stages[0]
	} else if completedIndex >= 0 && completedIndex+1 < len(stages) {
		expectedNext = stages[completedIndex+1]
	}
	if state.CompletedStage != completed || state.NextStage != expectedNext {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: checkpoint cursor identity mismatch")
	}
	return checkpoint, state, nil
}

func (e *Engine) verifyRestartCheckpoint(ctx context.Context, run model.Run, configuration model.RunConfiguration) (model.Checkpoint, cursorCheckpointState, error) {
	checkpoint, state, err := e.latestCursorCheckpoint(ctx, run.ID)
	if err != nil {
		return checkpoint, state, err
	}
	canonical, err := canonicalWorkspace(run.Project)
	if err != nil || canonical != run.Project || filepath.Clean(state.WorkspacePath) != canonical {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: canonical workspace identity changed")
	}
	if workspaceIdentityFingerprint(canonical) != state.WorkspaceIdentity || configuration.SHA256 != state.ConfigurationSHA {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: workspace or immutable configuration identity changed")
	}
	if state.GitHead == "" || state.GitIndex == "" {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: checkpoint has no verified Git HEAD and index")
	}
	_, managed, head, index, err := workspaceCheckpointHashes(canonical, "", nil)
	if err != nil {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: current workspace fingerprint failed")
	}
	if head != state.GitHead || index != state.GitIndex || !reflect.DeepEqual(managed, state.ManagedPathHashes) {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: Git HEAD, index, or managed paths changed")
	}
	for path, expected := range state.ArtifactPathHashes {
		current := map[string]string{}
		if err = addWorkspaceFileHash(canonical, path, current); err != nil || current[path] != expected {
			return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: checkpointed artifact changed: %s", path)
		}
	}
	if state.NextStage != "" && indexOfStage(state.NextStage) < 0 {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: checkpoint stage cursor is invalid")
	}
	if state.NextStage == "" {
		return checkpoint, state, fmt.Errorf("WORKSPACE_DRIFT: checkpoint has no resumable next stage")
	}
	return checkpoint, state, nil
}

func restoreLoopBudget(limits, counts LoopLimits) (*LoopBudget, error) {
	budget, err := NewLoopBudget(limits)
	if err != nil {
		return nil, err
	}
	if counts.AnalyzeRemediations < 0 || counts.AnalyzeRemediations > limits.AnalyzeRemediations || counts.TestRepairs < 0 || counts.TestRepairs > limits.TestRepairs || counts.ConvergenceRounds < 0 || counts.ConvergenceRounds > limits.ConvergenceRounds {
		return nil, fmt.Errorf("WORKSPACE_DRIFT: checkpoint loop counters exceed configured limits")
	}
	budget.analyzeRemediations = counts.AnalyzeRemediations
	budget.testRepairs = counts.TestRepairs
	budget.convergenceRounds = counts.ConvergenceRounds
	return budget, nil
}
