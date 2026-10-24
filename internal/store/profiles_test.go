package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/security"
	workspaceguard "github.com/spectide/spectide/internal/workspace"
)

func TestProfilesNeverStoreSecrets(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	_, e = s.SaveProfile(context.Background(), model.Profile{Name: "fake", Provider: "fake", Executable: "fake", AllowedScopes: []string{"personal"}})
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.Profiles(context.Background())
	if e != nil || len(got) != 1 {
		t.Fatalf("profiles=%v err=%v", got, e)
	}
	if got[0].ConfigDirectory != "" {
		t.Fatal("unexpected config")
	}
}

func TestProfileRejectsUnknownProviderAndShellPath(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := model.Profile{Name: "profile", Executable: "agent", AllowedScopes: []string{"project"}}
	base.Provider = "shell"
	if _, err = s.SaveProfile(context.Background(), base); err == nil {
		t.Fatal("unknown provider accepted")
	}
	base.Provider = "codex"
	base.Executable = "../agent"
	if _, err = s.SaveProfile(context.Background(), base); err == nil {
		t.Fatal("relative executable path accepted")
	}
	base.Executable = "codex"
	base.AllowedScopes = nil
	if _, err = s.SaveProfile(context.Background(), base); err == nil {
		t.Fatal("unscoped profile accepted")
	}
}

func TestRunStateTransitionsAndInterruptedRecovery(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.CreateRun(context.Background(), "demo", "safe task")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(context.Background(), r.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverInterrupted(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != model.Blocked {
		t.Fatalf("state=%s", got.State)
	}
	events, err := s.Events(context.Background(), r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].Type != "run.interrupted" {
		t.Fatalf("event=%+v", events[len(events)-1])
	}
	if err = s.SetRunState(context.Background(), r.ID, model.Completed); err == nil {
		t.Fatal("terminal state must be immutable")
	}
}

func TestDaemonRestartReconcilesAbandonedRun(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	project := t.TempDir()
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateRun(ctx, project, "interrupted provider execution")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO run_owners(run_id,owner_id,heartbeat_at) VALUES(?,?,?)`, run.ID, "dead-daemon", stale); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// Opening the state store after a daemon process restart preserves the
	// durable run for the daemon's startup recovery pass.
	restarted, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err = restarted.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err := restarted.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Blocked {
		t.Fatalf("abandoned run was not reconciled after restart: run=%+v err=%v", got, err)
	}
	events, err := restarted.Events(ctx, run.ID, 0)
	if err != nil || len(events) == 0 || events[len(events)-1].Type != "run.interrupted" {
		t.Fatalf("restart interruption evidence missing: events=%+v err=%v", events, err)
	}
}

func TestAbandonedPausedRunKeepsVerifiedResumeReservation(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	project := t.TempDir()
	run, err := s.CreateRun(ctx, project, "preserve a paused continuation")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "stopped-owner"); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, run.ID)
	var locks int
	if err == nil {
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_locks WHERE run_id=?`, run.ID).Scan(&locks)
	}
	if err != nil || got.State != model.Paused || locks != 1 {
		t.Fatalf("verified paused continuation was not retained: run=%+v locks=%d err=%v", got, locks, err)
	}
}

func TestAbandonedPausedCancellationWaitsForProcessFenceThenFinalizes(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	project := t.TempDir()
	run, err := s.CreateRun(ctx, project, "cancel an abandoned paused run")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkspaceLockOwned(ctx, project, run.ID, "stopped-owner"); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestRunControl(ctx, run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	processLock, err := workspaceguard.AcquireProcessLock(project)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	stillPaused, err := s.GetRun(ctx, run.ID)
	if err != nil || stillPaused.State != model.Paused {
		t.Fatalf("recovery crossed a live process fence: run=%+v err=%v", stillPaused, err)
	}
	if err = processLock.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Cancelled {
		t.Fatalf("abandoned cancellation was not finalized: run=%+v err=%v", got, err)
	}
	for _, table := range []string{"workspace_locks", "run_owners", "run_controls"} {
		var count int
		if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE run_id=?`, run.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s reservation survived terminal cancellation: count=%d err=%v", table, count, err)
		}
	}
	events, err := s.Events(ctx, run.ID, 0)
	if err != nil || len(events) < 2 || events[len(events)-1].Type != "run.cancelled" {
		t.Fatalf("cancellation audit evidence missing: events=%+v err=%v", events, err)
	}
	if err = s.RequestRunControl(ctx, run.ID, "cancel"); err != nil {
		t.Fatalf("repeated cancellation was not idempotent: %v", err)
	}
	reused, err := s.CreateRun(ctx, project, "reuse workspace after cancellation")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkspaceLockOwned(ctx, project, reused.ID, "new-owner"); err != nil {
		t.Fatalf("terminal cancellation did not release workspace for reuse: %v", err)
	}
}

func TestPendingCancellationCannotBeOverwrittenByResume(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, err := s.CreateRun(ctx, t.TempDir(), "keep cancellation sticky")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Paused); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestRunControl(ctx, run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestRunControl(ctx, run.ID, "resume"); err == nil || !strings.Contains(err.Error(), "RUN_CANCEL_PENDING") {
		t.Fatalf("resume overwrote accepted cancellation: %v", err)
	}
}

func TestAcceptedCancellationWinsTerminalCompletionRace(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, err := s.CreateRun(ctx, t.TempDir(), "cancel at the completion boundary")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, run.ID, model.Running); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestRunControl(ctx, run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeRunOutcome(ctx, run.ID, model.Completed, "run.completed", "info", map[string]any{"execution": "LIVE", "verdict": "PASS"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Cancelled {
		t.Fatalf("accepted cancellation lost to completion: run=%+v err=%v", got, err)
	}
	events, err := s.Events(ctx, run.ID, 0)
	requested, cancelled, terminal := -1, -1, -1
	for i, event := range events {
		if event.Type == "run.cancel_requested" {
			requested = i
		}
		if event.Type == "run.cancelled" {
			cancelled = i
			if event.Payload["verdict"] != "CANCELLED" {
				t.Fatalf("cancellation event retained successful verdict: %+v", event)
			}
		}
		if event.Type == "run.CANCELLED" {
			terminal = i
		}
		if event.Type == "run.completed" {
			t.Fatalf("successful outcome event bypassed accepted cancellation: %+v", event)
		}
	}
	if err != nil || requested < 0 || cancelled <= requested || terminal <= cancelled {
		t.Fatalf("cancellation/completion events are not ordered: events=%+v err=%v", events, err)
	}
}

func TestAbandonedCreatedRunIsRecoveredWithoutChangingIdempotencyIdentity(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	project := t.TempDir()
	configuration := []byte(`{"route":"fake"}`)
	run, reused, err := s.CreateRunWithConfigurationOwned(ctx, "launch-once", project, "recover before worker start", configuration, "launch-owner")
	if err != nil || reused {
		t.Fatalf("run=%+v reused=%v err=%v", run, reused, err)
	}
	stale := time.Now().UTC().Add(-RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestRunControl(ctx, run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Cancelled {
		t.Fatalf("abandoned created cancellation not recovered: run=%+v err=%v", got, err)
	}
	retried, reused, err := s.CreateRunWithConfigurationOwned(ctx, "launch-once", project, "recover before worker start", configuration, "retry-owner")
	if err != nil || !reused || retried.ID != run.ID || retried.State != model.Cancelled {
		t.Fatalf("idempotent retry changed or replayed the original launch: run=%+v reused=%v err=%v", retried, reused, err)
	}
	events, err := s.Events(ctx, run.ID, 0)
	if err != nil || len(events) < 3 || events[len(events)-1].Type != "run.cancelled" {
		t.Fatalf("created-run cancellation evidence missing: events=%+v err=%v", events, err)
	}
}

func TestCreatedLaunchRecoveryWaitsForOwnerLeaseAndProcessFence(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, _, err := s.CreateRunWithConfigurationOwned(ctx, "healthy-launch", t.TempDir(), "keep healthy launch", []byte(`{}`), "active-launcher")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Created {
		t.Fatalf("fresh launch lease was treated as abandoned: run=%+v err=%v", got, err)
	}
	projectRun, err := s.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := workspaceguard.AcquireProcessLock(projectRun.Project)
	if err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-RunOwnerLease - time.Second).Format(time.RFC3339Nano)
	if _, err = s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Created {
		t.Fatalf("recovery crossed a healthy launch process fence: run=%+v err=%v", got, err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverAbandonedRuns(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetRun(ctx, run.ID)
	if err != nil || got.State != model.Blocked {
		t.Fatalf("expired launch owner was not blocked after its process exited: run=%+v err=%v", got, err)
	}
}

func TestVersionedSchemaContainsRecoveryEntities(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	if err = s.DB.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("schema version=%d", version)
	}
	for _, table := range []string{"projects", "routing_decisions", "artifacts", "checkpoints", "approvals", "workspace_locks", "idempotency_keys", "run_controls", "run_configurations", "run_owners", "profile_capabilities", "execution_attempts", "quality_executions", "quality_results", "quality_gates"} {
		var found string
		err = s.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&found)
		if err != nil {
			t.Fatalf("missing table %s: %v", table, err)
		}
	}
}

func TestProfileCapabilityPersistenceAndInvalidation(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	profile := model.Profile{Name: "local", Provider: "codex", Executable: "/bin/sh", AllowedScopes: []string{"/workspace"}}
	if _, err = s.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	reference, err := security.ResolveExecutableReference(profile.Executable)
	if err != nil {
		t.Fatal(err)
	}
	capability := model.ProfileCapability{Provider: "codex", ExecutableFound: true, ApprovedExecutablePath: reference.Path, ExecutableSHA256: reference.SHA256, Version: "1.2.3", Authenticated: "NOT_PROBED", Warnings: []string{"token=supersecret123456"}}
	badReference := capability
	badReference.ExecutableSHA256 = strings.Repeat("a", 64)
	if _, err = s.SaveProfileCapability(ctx, profile.Name, badReference); err == nil {
		t.Fatal("mismatched executable identity was persisted as capability evidence")
	}
	if _, err = s.SaveProfileCapability(ctx, profile.Name, capability); err != nil {
		t.Fatal(err)
	}
	profiles, err := s.Profiles(ctx)
	if err != nil || len(profiles) != 1 || profiles[0].Capability == nil {
		t.Fatalf("profiles=%+v err=%v", profiles, err)
	}
	if profiles[0].Capability.Authenticated != "NOT_PROBED" || strings.Contains(strings.Join(profiles[0].Capability.Warnings, " "), "supersecret") {
		t.Fatalf("capability was not safely persisted: %+v", profiles[0].Capability)
	}
	if profiles[0].Capability.ApprovedExecutablePath != capability.ApprovedExecutablePath || profiles[0].Capability.ExecutableSHA256 != capability.ExecutableSHA256 {
		t.Fatalf("approved executable identity was not persisted: %+v", profiles[0].Capability)
	}
	if err = s.InvalidateProfileCapability(ctx, profile.Name); err != nil {
		t.Fatal(err)
	}
	profiles, err = s.Profiles(ctx)
	if err != nil || profiles[0].Capability != nil {
		t.Fatalf("reference-drift invalidation retained stale capability: profiles=%+v err=%v", profiles, err)
	}
	if _, err = s.SaveProfileCapability(ctx, profile.Name, capability); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	profiles, err = s.Profiles(ctx)
	if err != nil || profiles[0].Capability != nil {
		t.Fatalf("edited profile retained stale capability: profiles=%+v err=%v", profiles, err)
	}
	capability.Provider = "claude"
	if _, err = s.SaveProfileCapability(ctx, profile.Name, capability); err == nil {
		t.Fatal("provider-mismatched capability accepted")
	}
}

func TestIdempotencyKeyReusesOnlyTheSameRunRequest(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	first, reused, err := s.CreateRunIdempotent(ctx, "request-key-123", "/workspace", "same prompt")
	if err != nil || reused {
		t.Fatalf("first=%+v reused=%t err=%v", first, reused, err)
	}
	second, reused, err := s.CreateRunIdempotent(ctx, "request-key-123", "/workspace", "same prompt")
	if err != nil || !reused || first.ID != second.ID {
		t.Fatalf("second=%+v reused=%t err=%v", second, reused, err)
	}
	if _, _, err = s.CreateRunIdempotent(ctx, "request-key-123", "/workspace", "different prompt"); err == nil {
		t.Fatal("key reused for a different payload")
	}
	runs, err := s.Runs(ctx)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%d err=%v", len(runs), err)
	}
}

func TestEventPagesStayWithinCapturedHighWater(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	run, err := s.CreateRun(ctx, "demo", "pagination")
	if err != nil {
		t.Fatal(err)
	}
	afterInitialization, err := s.EventHighWater(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err = s.Event(ctx, run.ID, "stage", "event", "info", map[string]any{"index": i}); err != nil {
			t.Fatal(err)
		}
	}
	high, err := s.EventHighWater(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Event(ctx, run.ID, "stage", "later", "info", nil); err != nil {
		t.Fatal(err)
	}
	page, err := s.EventsPage(ctx, run.ID, afterInitialization, high, 2)
	if err != nil || len(page) != 2 || page[1].Seq != afterInitialization+2 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	page, err = s.EventsPage(ctx, run.ID, page[1].Seq, high, 20)
	if err != nil || len(page) != 2 || page[len(page)-1].Seq != high {
		t.Fatalf("remainder=%+v err=%v", page, err)
	}
}

func TestPromptsAndNestedEventsAreRedacted(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.CreateRun(context.Background(), "demo", "token=secret-value-123456789")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Prompt, "secret-value") {
		t.Fatalf("prompt not redacted: %s", r.Prompt)
	}
	if err = s.Event(context.Background(), r.ID, "", "test.secret", "info", map[string]any{"nested": map[string]any{"token": "secret-value-123456789"}}); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events(context.Background(), r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(events[len(events)-1].Payload["nested"].(map[string]any)["token"].(string), "secret-value") {
		t.Fatal("event secret not redacted")
	}
}
