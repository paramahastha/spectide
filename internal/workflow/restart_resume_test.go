package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
)

func TestRestartResumeRequiresAndRestoresVerifiedCheckpoint(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	configurationJSON := fakeRunConfiguration(t)
	run, reused, err := s.CreateRunWithConfiguration(ctx, "", project, "restart safely", configurationJSON)
	if err != nil || reused {
		t.Fatalf("create run reused=%v err=%v", reused, err)
	}
	owner := New(s)
	configuration, err := s.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = s.Event(ctx, run.ID, stages[0], "run.paused", "info", map[string]any{"stage": stages[0]}); err != nil {
		t.Fatal(err)
	}
	route := config.Route{Provider: "fake", Profile: "fake-demo"}
	if err = s.AddStage(ctx, model.Stage{ID: "interrupted-stage-attempt", RunID: run.ID, Key: stages[0], Position: 0, State: model.Pending, Provider: route.Provider, Profile: route.Profile, Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetStage(ctx, "interrupted-stage-attempt", model.StageRun); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: run.ID, Stage: stages[0], Phase: "approval_check", Provider: route.Provider, Profile: route.Profile}); err != nil {
		t.Fatal(err)
	}
	spent := LoopLimits{AnalyzeRemediations: 1, TestRepairs: 2, ConvergenceRounds: 3}
	cursor := checkpointCursor{Phase: "approval", Operation: "continue_after_resolution", Tasks: []string{"T087"}, Check: "unit", Position: 2}
	if err = owner.saveCursorCheckpoint(ctx, run, "", "starting", stages[0], configuration.SHA256, "SIMULATED", spent, cursor); err != nil {
		t.Fatal(err)
	}
	checkpointRows, err := s.Checkpoints(ctx, run.ID)
	if err != nil || len(checkpointRows) == 0 {
		t.Fatalf("load continuation checkpoint rows=%d err=%v", len(checkpointRows), err)
	}
	var savedCursor cursorCheckpointState
	if err = json.Unmarshal([]byte(checkpointRows[len(checkpointRows)-1].StateJSON), &savedCursor); err != nil {
		t.Fatal(err)
	}
	if savedCursor.LoopCounts != spent || savedCursor.PendingPhase != cursor.Phase || savedCursor.PendingOperation != cursor.Operation || savedCursor.PendingCheck != cursor.Check || savedCursor.TaskPosition != cursor.Position || len(savedCursor.PendingTasks) != 1 || savedCursor.PendingTasks[0] != "T087" {
		t.Fatalf("checkpoint did not persist precise continuation state: %+v", savedCursor)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = New(s).Resume(run.ID); err != nil {
		t.Fatalf("verified restart resume: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resumed, getErr := s.GetRun(ctx, run.ID)
		if getErr == nil && resumed.State == model.Completed {
			events, eventErr := s.Events(ctx, run.ID, 0)
			if eventErr != nil {
				t.Fatal(eventErr)
			}
			for _, event := range events {
				if event.Type == "run.recovery_claimed" {
					resumedStages, stagesErr := s.Stages(ctx, run.ID)
					if stagesErr != nil {
						t.Fatal(stagesErr)
					}
					var prior, replay model.Stage
					for _, stage := range resumedStages {
						if stage.Key == stages[0] && stage.Attempt == 1 {
							prior = stage
						}
						if stage.Key == stages[0] && stage.Attempt == 2 {
							replay = stage
						}
					}
					if prior.State != model.StageFail || replay.State != model.Succeeded {
						t.Fatalf("ambiguous stage attempt was not finalized before numbered replay: prior=%+v replay=%+v", prior, replay)
					}
					attempts, attemptErr := s.ExecutionAttempts(ctx, run.ID)
					if attemptErr != nil || len(attempts) != 1 || attempts[0].Result != "FAILED" || !strings.Contains(attempts[0].Error, "ambiguous") {
						t.Fatalf("ambiguous provider invocation was not finalized honestly: attempts=%+v err=%v", attempts, attemptErr)
					}
					return
				}
			}
			t.Fatal("completed resumed run omitted recovery-claimed evidence")
		}
		time.Sleep(15 * time.Millisecond)
	}
	resumed, _ := s.GetRun(ctx, run.ID)
	t.Fatalf("resumed workflow did not complete, state=%s", resumed.State)
}

func TestCheckpointTracksLatestArtifactVersionsAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	feature := filepath.Join(project, "specs", "001-product")
	if err := os.MkdirAll(feature, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
		if err := os.WriteFile(filepath.Join(feature, name), []byte("final "+name+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sourcePath := filepath.Join(project, "source.go")
	if err := os.WriteFile(sourcePath, []byte("package source // final\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, _, err := s.CreateRunWithConfiguration(ctx, "", project, "checkpoint successive artifact revisions", fakeRunConfiguration(t))
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"specs/001-product/spec.md", "specs/001-product/tasks.md", "source.go"}
	baseTime := time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)
	for _, path := range paths {
		oldContent := []byte("historical " + path + "\n")
		currentPath := filepath.Join(project, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(currentPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(currentPath, oldContent, 0600); err != nil {
			t.Fatal(err)
		}
		oldDigest := sha256.Sum256(oldContent)
		if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "specify", Path: path, Kind: "text", SHA256: hex.EncodeToString(oldDigest[:]), ObservedAt: baseTime}); err != nil {
			t.Fatal(err)
		}
		finalContent := []byte("final " + path + "\n")
		if path == "source.go" {
			finalContent = []byte("package source // final\n")
		}
		if err = os.WriteFile(currentPath, finalContent, 0600); err != nil {
			t.Fatal(err)
		}
		finalDigest := sha256.Sum256(finalContent)
		if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "implement", Path: path, Kind: "text", SHA256: hex.EncodeToString(finalDigest[:]), ObservedAt: baseTime.Add(time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	artifacts, err := s.Artifacts(ctx, run.ID)
	if err != nil || len(artifacts) != 6 {
		t.Fatalf("historical artifact observations were not preserved: count=%d err=%v", len(artifacts), err)
	}
	configuration, err := s.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	owner := New(s)
	if err = owner.saveCursorCheckpoint(ctx, run, feature, "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
		t.Fatalf("checkpoint rejected latest artifact revisions: %v", err)
	}
	if _, _, err = owner.verifyRestartCheckpoint(ctx, run, configuration); err != nil {
		t.Fatalf("latest-version checkpoint did not verify for restart: %v", err)
	}
	if err = os.WriteFile(sourcePath, []byte("external drift after checkpoint\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = owner.verifyRestartCheckpoint(ctx, run, configuration); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
		t.Fatalf("genuine post-checkpoint drift was not rejected: %v", err)
	}
}

func TestCLISubprocessResumeWaitsForDurableTerminalEvidence(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	dbPath := filepath.Join(t.TempDir(), "state.db")
	state, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	run, reused, err := state.CreateRunWithConfiguration(ctx, "", project, "resume in a foreground CLI", fakeRunConfiguration(t))
	if err != nil || reused {
		t.Fatalf("run=%+v reused=%v err=%v", run, reused, err)
	}
	if err = state.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = state.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = state.Event(ctx, run.ID, stages[0], "run.paused", "info", map[string]any{"stage": stages[0]}); err != nil {
		t.Fatal(err)
	}
	configuration, err := state.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = New(state).saveCursorCheckpoint(ctx, run, "", "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = state.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(t.TempDir(), "spectide")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/spectide")
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		t.Fatalf("build CLI: %v: %s", buildErr, output)
	}
	started := time.Now()
	command := exec.Command(binary, "--db", dbPath, "resume", run.ID)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resume CLI: %v: %s", err, output)
	}
	if elapsed := time.Since(started); elapsed < 500*time.Millisecond {
		t.Fatalf("resume CLI exited before foreground work completed (%s): %s", elapsed, output)
	}

	state, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	completed, err := state.GetRun(ctx, run.ID)
	if err != nil || completed.State != model.Completed {
		t.Fatalf("resumed run=%+v err=%v", completed, err)
	}
	report, err := state.Report(ctx, run.ID)
	if err != nil || report.Acceptance.Verdict != "SIMULATED" {
		t.Fatalf("terminal report acceptance=%+v err=%v", report.Acceptance, err)
	}
	events, err := state.Events(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	claimed, terminal := false, false
	for _, event := range events {
		if event.Type == "run.recovery_claimed" {
			claimed = true
		}
		if event.Type == "run.COMPLETED" {
			terminal = true
		}
	}
	if !claimed || !terminal {
		t.Fatalf("resumed terminal events missing: recovery=%v completed=%v", claimed, terminal)
	}
}

func TestRestartResumeBlocksOnManagedWorkspaceDriftWithoutOverwriting(t *testing.T) {
	ctx := context.Background()
	project := initCheckpointGitProject(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	configurationJSON := fakeRunConfiguration(t)
	run, _, err := s.CreateRunWithConfiguration(ctx, "", project, "preserve drift evidence", configurationJSON)
	if err != nil {
		t.Fatal(err)
	}
	owner := New(s)
	configuration, err := s.RunConfiguration(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = s.Event(ctx, run.ID, stages[0], "run.paused", "info", map[string]any{"stage": stages[0]}); err != nil {
		t.Fatal(err)
	}
	if err = owner.saveCursorCheckpoint(ctx, run, "", "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(project, "managed.txt")
	if err = os.WriteFile(artifact, []byte("changed after checkpoint\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = New(s).Resume(run.ID); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
		t.Fatalf("drifted restart resume err=%v", err)
	}
	got, err := s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Blocked {
		t.Fatalf("drifted run=%+v err=%v", got, err)
	}
	content, err := os.ReadFile(artifact)
	if err != nil || string(content) != "changed after checkpoint\n" {
		t.Fatalf("recovery overwrote drift evidence %q err=%v", content, err)
	}
}

func TestRestartResumeBlocksOnRegisteredArtifactAndGitHeadDrift(t *testing.T) {
	for _, drift := range []string{"registered artifact content", "Git HEAD", "checkpoint identity", "immutable configuration hash"} {
		t.Run(drift, func(t *testing.T) {
			ctx := context.Background()
			project := initCheckpointGitProject(t)
			s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			run, _, err := s.CreateRunWithConfiguration(ctx, "", project, "preserve registered evidence", fakeRunConfiguration(t))
			if err != nil {
				t.Fatal(err)
			}
			owner := New(s)
			configuration, err := s.RunConfiguration(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "expired-owner"); err != nil {
				t.Fatal(err)
			}
			if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
				t.Fatal(err)
			}
			if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
				t.Fatal(err)
			}
			if err = s.Event(ctx, run.ID, stages[0], "run.paused", "info", map[string]any{"stage": stages[0]}); err != nil {
				t.Fatal(err)
			}
			artifactPath := filepath.Join(project, "generated", "evidence.txt")
			if drift == "registered artifact content" {
				if err = os.MkdirAll(filepath.Dir(artifactPath), 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(artifactPath, []byte("registered content\n"), 0600); err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256([]byte("registered content\n"))
				if _, err = s.SaveArtifact(ctx, model.Artifact{RunID: run.ID, Stage: "specify", Path: "generated/evidence.txt", Kind: "text", SHA256: hex.EncodeToString(digest[:])}); err != nil {
					t.Fatal(err)
				}
			}
			if err = owner.saveCursorCheckpoint(ctx, run, "", "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
				t.Fatal(err)
			}
			if drift == "registered artifact content" {
				if err = os.WriteFile(artifactPath, []byte("changed artifact content\n"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if drift == "Git HEAD" {
				gitTestCommand(t, project, "commit", "--allow-empty", "-qm", "head drift")
			} else if drift == "checkpoint identity" || drift == "immutable configuration hash" {
				checkpoints, listErr := s.Checkpoints(ctx, run.ID)
				if listErr != nil || len(checkpoints) == 0 {
					t.Fatalf("checkpoint list=%v err=%v", checkpoints, listErr)
				}
				var state map[string]any
				if err = json.Unmarshal([]byte(checkpoints[len(checkpoints)-1].StateJSON), &state); err != nil {
					t.Fatal(err)
				}
				if drift == "checkpoint identity" {
					state["checkpoint_id"] = "different-checkpoint"
				} else {
					state["configuration_sha256"] = strings.Repeat("0", 64)
				}
				encoded, marshalErr := json.Marshal(state)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if _, err = s.DB.ExecContext(ctx, `UPDATE checkpoints SET state_json=? WHERE id=?`, string(encoded), checkpoints[len(checkpoints)-1].ID); err != nil {
					t.Fatal(err)
				}
			}
			stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
			if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
				t.Fatal(err)
			}
			if err = New(s).Resume(run.ID); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
				t.Fatalf("drifted restart resume err=%v", err)
			}
			got, err := s.GetRun(ctx, run.ID)
			if err != nil || got.State != model.Blocked {
				t.Fatalf("drifted run=%+v err=%v", got, err)
			}
			if drift == "registered artifact content" {
				content, readErr := os.ReadFile(artifactPath)
				if readErr != nil || string(content) != "changed artifact content\n" {
					t.Fatalf("recovery overwrote artifact evidence %q err=%v", content, readErr)
				}
			}
		})
	}
}

func fakeRunConfiguration(t *testing.T) []byte {
	t.Helper()
	type configuredStage struct {
		Stage     string          `json:"stage"`
		Decision  config.Decision `json:"decision"`
		Simulated bool            `json:"simulated"`
	}
	var configured []configuredStage
	for _, stage := range stages {
		decision := config.Decision{Stage: stage, Route: config.Route{Provider: "fake", Profile: "fake-demo"}, Mode: "hybrid", Explanation: "fixture"}
		configured = append(configured, configuredStage{Stage: stage, Decision: decision, Simulated: true})
	}
	encoded, err := json.Marshal(configured)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func initCheckpointGitProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	gitTestCommand(t, project, "init", "-q")
	gitTestCommand(t, project, "config", "user.email", "spectide-test@example.invalid")
	gitTestCommand(t, project, "config", "user.name", "SpecTide Test")
	if err := os.WriteFile(filepath.Join(project, ".gitignore"), []byte("generated/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "managed.txt"), []byte("checkpoint content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, project, "add", ".gitignore", "managed.txt")
	gitTestCommand(t, project, "commit", "-qm", "checkpoint fixture")
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func gitTestCommand(t *testing.T, project string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", project}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
