package workflow

import (
	"context"
	"fmt"
	"github.com/spectide/spectide/internal/config"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFakeLifecyclePersistsEvidence(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "spectide.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	eng := New(s)
	r, e := eng.Start(context.Background(), t.TempDir(), "Build a test")
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := s.GetRun(context.Background(), r.ID)
		if got.State == model.Completed {
			rep, e := s.Report(context.Background(), r.ID)
			if e != nil {
				t.Fatal(e)
			}
			if len(rep.Stages) != 12 || len(rep.Decisions) != 12 || len(rep.Gates) != 5 {
				t.Fatalf("stages=%d decisions=%d gates=%d", len(rep.Stages), len(rep.Decisions), len(rep.Gates))
			}
			configuration, err := s.RunConfiguration(context.Background(), r.ID)
			if err != nil || len(configuration.SHA256) != 64 || !strings.Contains(configuration.ConfigJSON, `"simulated":true`) {
				t.Fatalf("effective run configuration=%+v err=%v", configuration, err)
			}
			if rep.Configuration == nil || rep.Configuration.SHA256 != configuration.SHA256 {
				t.Fatalf("report omitted immutable run configuration: %+v", rep.Configuration)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("workflow did not complete")
}

func TestTwentyDeterministicRunsCompleteWithRequiredEvidence(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "spectide.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	eng := New(s)
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		project := filepath.Join(root, fmt.Sprintf("project-%02d", i))
		if err := os.Mkdir(project, 0o700); err != nil {
			t.Fatalf("create project %d: %v", i, err)
		}
		run, err := eng.Start(context.Background(), project, fmt.Sprintf("deterministic acceptance run %d", i))
		if err != nil {
			t.Fatalf("start run %d: %v", i, err)
		}
		result, err := eng.Wait(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("wait run %d: %v", i, err)
		}
		if result.State != model.Completed {
			t.Fatalf("run %d ended %s", i, result.State)
		}
		report, err := s.Report(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("report run %d: %v", i, err)
		}
		if len(report.Stages) != 12 || len(report.Decisions) != 12 || len(report.Gates) != 5 {
			t.Fatalf("run %d evidence counts stages=%d decisions=%d gates=%d", i, len(report.Stages), len(report.Decisions), len(report.Gates))
		}
		for _, gate := range report.Gates {
			if gate.Verdict != "PASS" || !strings.Contains(strings.ToLower(gate.Evidence), "simulated") {
				t.Fatalf("run %d has unevidenced simulated gate: %+v", i, gate)
			}
		}
	}
}

func TestOnlyOneActiveWriterPerWorkspace(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "spectide.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	eng := New(s)
	project := t.TempDir()
	first, e := eng.Start(context.Background(), project, "first")
	if e != nil {
		t.Fatal(e)
	}
	second, e := eng.Start(context.Background(), project, "second")
	if e != nil {
		t.Fatal(e)
	}
	if first.ID == second.ID {
		t.Fatal("runs share an ID")
	}
	got, e := s.GetRun(context.Background(), second.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.State != model.Blocked {
		t.Fatalf("second run state=%s", got.State)
	}
	if _, e = eng.Wait(context.Background(), first.ID); e != nil {
		t.Fatal(e)
	}
	third, e := eng.Start(context.Background(), project, "third")
	if e != nil {
		t.Fatal(e)
	}
	got, e = s.GetRun(context.Background(), third.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.State == model.Blocked {
		t.Fatal("workspace lock was not released after completion")
	}
}

func TestStartRequiresExistingDirectoryAndEnforcesConfiguredScopes(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	engine := New(s)

	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := engine.Start(context.Background(), missing, "must reject missing path"); err == nil || !strings.Contains(err.Error(), "PROJECT_PATH_UNAVAILABLE") {
		t.Fatalf("missing project error=%v", err)
	}

	allowed := t.TempDir()
	outside := t.TempDir()
	cfg := config.Default()
	cfg.AllowedScopes = []string{allowed}
	if err := config.Save(config.ProjectPath(outside), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(context.Background(), outside, "must reject outside scope"); err == nil || !strings.Contains(err.Error(), "PERMISSION_DENIED") {
		t.Fatalf("outside-scope error=%v", err)
	}
	if runs, err := s.Runs(context.Background()); err != nil || len(runs) != 0 {
		t.Fatalf("rejected workspace created runs=%d err=%v", len(runs), err)
	}
}

func TestStartCanonicalizesSymlinkWithinAllowedScope(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "project-link")
	if err := os.Symlink(project, link); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.AllowedScopes = []string{root}
	if err := config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	run, err := New(s).Start(context.Background(), link, "canonical path")
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if run.Project != canonical {
		t.Fatalf("run project=%q, want canonical path %q", run.Project, canonical)
	}
}

func TestEngineUsesConfiguredSimulatedRouteForEveryStage(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	project := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute.Profile = "fixture-personal"
	cfg.DefaultRoute.Model = "fixture-model"
	cfg.DefaultRoute.Effort = "low"
	if err = config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	e := New(s)
	r, err := e.Start(context.Background(), project, "test route persistence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Wait(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	report, err := s.Report(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Stages) != 12 || len(report.Decisions) != 12 {
		t.Fatalf("stages=%d decisions=%d", len(report.Stages), len(report.Decisions))
	}
	for _, st := range report.Stages {
		if st.Profile != "fixture-personal" || st.Model != "fixture-model" || st.Effort != "low" {
			t.Fatalf("route not applied: %+v", st)
		}
	}
	for _, d := range report.Decisions {
		if d.Policy != "hybrid" || d.Explanation != "hybrid default route" {
			t.Fatalf("decision=%+v", d)
		}
	}
}

func TestEngineDoesNotSilentlySimulateLiveRoute(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	project := t.TempDir()
	cfg := config.Default()
	cfg.DefaultRoute.Provider = "codex"
	cfg.DefaultRoute.Profile = "work"
	if err = config.Save(config.ProjectPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err = New(s).Start(context.Background(), project, "task"); err == nil || !strings.Contains(err.Error(), "LIVE_STAGE_EXECUTION_UNAVAILABLE") {
		t.Fatalf("err=%v", err)
	}
	runs, err := s.Runs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("live configuration created a run that it cannot execute: %+v", runs)
	}
}

func TestRunCanPauseResumeAndCancelFromAnotherEngine(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	owner := New(s)
	control := New(s) // Models a separate CLI process sharing the durable store.
	run, err := owner.Start(context.Background(), t.TempDir(), "pause and resume")
	if err != nil {
		t.Fatal(err)
	}
	waitForState := func(id string, state model.RunState) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			got, getErr := s.GetRun(context.Background(), id)
			if getErr == nil && got.State == state {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		got, _ := s.GetRun(context.Background(), id)
		t.Fatalf("run %s state=%s, expected %s", id, got.State, state)
	}
	waitForState(run.ID, model.Running)
	if err = control.Pause(run.ID); err != nil {
		t.Fatal(err)
	}
	waitForState(run.ID, model.Paused)
	if err = control.Resume(run.ID); err != nil {
		t.Fatal(err)
	}
	waitForState(run.ID, model.Running)

	toCancel, err := owner.Start(context.Background(), t.TempDir(), "cross process cancellation")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(toCancel.ID, model.Running)
	if err = control.Cancel(context.Background(), toCancel.ID); err != nil {
		t.Fatal(err)
	}
	waitForState(toCancel.ID, model.Cancelled)
	if _, err = owner.Wait(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
}

func TestResumeAfterOwnerLeaseExpiryBlocksWithWorkspaceDrift(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	project := t.TempDir()
	run, err := s.CreateRun(ctx, project, "interrupted paused run")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "dead-owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-store.RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	controller := New(s)
	if err = controller.Resume(run.ID); err == nil || !strings.Contains(err.Error(), "WORKSPACE_DRIFT") {
		t.Fatalf("stale owner resume error=%v", err)
	}
	got, err := s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Blocked {
		t.Fatalf("recovered run=%+v err=%v", got, err)
	}
	events, err := s.Events(ctx, run.ID, 0)
	if err != nil || events[len(events)-1].Type != "run.interrupted" || !strings.Contains(events[len(events)-1].Payload["reason"].(string), "WORKSPACE_DRIFT") {
		t.Fatalf("recovery events=%+v err=%v", events, err)
	}
	var locks int
	if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_locks WHERE run_id=?`, run.ID).Scan(&locks); err != nil || locks != 0 {
		t.Fatalf("stale workspace lock count=%d err=%v", locks, err)
	}
}
