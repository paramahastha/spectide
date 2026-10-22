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
	if err = owner.saveCursorCheckpoint(ctx, run, "", "starting", stages[0], configuration.SHA256, "SIMULATED", LoopLimits{}); err != nil {
		t.Fatal(err)
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
