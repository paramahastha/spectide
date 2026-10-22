package store

import (
	"context"
	"github.com/spectide/spectide/internal/model"
	"path/filepath"
	"strings"
	"testing"
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
	if version != 9 {
		t.Fatalf("schema version=%d", version)
	}
	for _, table := range []string{"projects", "routing_decisions", "artifacts", "checkpoints", "approvals", "workspace_locks", "idempotency_keys", "run_controls", "run_configurations", "run_owners", "profile_capabilities", "execution_attempts", "quality_executions", "quality_results"} {
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
	profile := model.Profile{Name: "local", Provider: "codex", Executable: "codex", AllowedScopes: []string{"/workspace"}}
	if _, err = s.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	capability := model.ProfileCapability{Provider: "codex", ExecutableFound: true, Version: "1.2.3", Authenticated: "NOT_PROBED", Warnings: []string{"token=supersecret123456"}}
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
	page, err := s.EventsPage(ctx, run.ID, 0, high, 2)
	if err != nil || len(page) != 2 || page[1].Seq != 2 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	page, err = s.EventsPage(ctx, run.ID, page[1].Seq, high, 20)
	if err != nil || len(page) != 3 || page[len(page)-1].Seq != high {
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
