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
	if version != 1 {
		t.Fatalf("schema version=%d", version)
	}
	for _, table := range []string{"projects", "routing_decisions", "artifacts", "checkpoints", "approvals", "workspace_locks"} {
		var found string
		err = s.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&found)
		if err != nil {
			t.Fatalf("missing table %s: %v", table, err)
		}
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
