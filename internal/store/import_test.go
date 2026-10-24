package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestImportLegacyWorkspaceBacksUpRetainsAndNeverOverwrites(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "legacy.db")
	legacy, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(dir, "project")
	if err = os.Mkdir(projectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = legacy.RegisterProject(ctx, projectPath, "project", []string{projectPath}); err != nil {
		t.Fatal(err)
	}
	run, err := legacy.CreateRun(ctx, projectPath, "legacy prompt")
	if err != nil {
		t.Fatal(err)
	}
	// Rewind only the additive shared-workspace migration so the fixture has
	// the schema shape of the prior release.
	for _, statement := range []string{
		`DROP TABLE workspace_import_events`,
		`DELETE FROM schema_migrations WHERE version=14`,
		`DROP INDEX runs_project_history`,
		`ALTER TABLE runs DROP COLUMN project_id`,
		`DROP TABLE workspace_imports`,
		`DROP TABLE workspace_metadata`,
		`DELETE FROM schema_migrations WHERE version=13`,
	} {
		if _, err = legacy.DB.Exec(statement); err != nil {
			t.Fatalf("rewind legacy fixture with %q: %v", statement, err)
		}
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(dir, "new", "spectide.db")
	backup, err := ImportLegacyWorkspace(ctx, source, destination, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(source); err != nil {
		t.Fatalf("source was not retained: %v", err)
	}
	backupInfo, err := os.Stat(backup)
	if err != nil || backupInfo.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions=%v err=%v", backupInfo, err)
	}
	imported, err := Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Close()
	got, err := imported.GetRun(ctx, run.ID)
	if err != nil || got.Prompt != "legacy prompt" {
		t.Fatalf("imported run=%+v err=%v", got, err)
	}
	var status string
	if err = imported.DB.QueryRow(`SELECT status FROM workspace_imports`).Scan(&status); err != nil || status != "COMPLETED" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	var backupReference string
	if err = imported.DB.QueryRow(`SELECT backup_reference FROM workspace_imports`).Scan(&backupReference); err != nil || backupReference != backup {
		t.Fatalf("backup reference=%q err=%v", backupReference, err)
	}
	events, err := imported.WorkspaceImportEvents(ctx, func() string {
		var id string
		_ = imported.DB.QueryRow(`SELECT id FROM workspace_imports`).Scan(&id)
		return id
	}())
	wantEvents := []string{"DETECTED", "AWAITING_CONFIRMATION", "BACKED_UP", "RESTORED", "VALIDATED", "COMPLETED"}
	if err != nil || len(events) != len(wantEvents) {
		t.Fatalf("import lifecycle events=%+v err=%v", events, err)
	}
	for index, status := range wantEvents {
		if events[index].Sequence != index+1 || events[index].Status != status {
			t.Fatalf("import lifecycle event %d=%+v want %s", index, events[index], status)
		}
	}
	if _, err = ImportLegacyWorkspace(ctx, source, destination, true); err == nil {
		t.Fatal("expected destination overwrite refusal")
	}
}

func TestImportLegacyRejectsCorruptAndUnsupportedSourcesWithoutDestinationChanges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "corrupt.db")
	if err := os.WriteFile(corrupt, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	unsupported := filepath.Join(dir, "future.db")
	future, err := sql.Open("sqlite", unsupported)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = future.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL); INSERT INTO schema_migrations VALUES(999,'future')`); err != nil {
		t.Fatal(err)
	}
	if err = future.Close(); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{corrupt, unsupported} {
		destination := filepath.Join(dir, filepath.Base(source)+"-destination", "state.db")
		if _, err = ImportLegacyWorkspace(ctx, source, destination, true); err == nil {
			t.Fatalf("accepted invalid source %s", source)
		}
		if _, statErr := os.Lstat(destination); !os.IsNotExist(statErr) {
			t.Fatalf("invalid source modified destination %s: %v", destination, statErr)
		}
		if _, statErr := os.Stat(source); statErr != nil {
			t.Fatalf("invalid source was changed or removed: %v", statErr)
		}
	}
}

func TestImportLegacyRequiresExplicitConfirmation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	if err := os.WriteFile(source, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "new", "state.db")
	if _, err := ImportLegacyWorkspace(ctx, source, destination, false); err == nil || err.Error() != "IMPORT_CONFIRMATION_REQUIRED" {
		t.Fatalf("unconfirmed import error=%v", err)
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("unconfirmed import changed source %q err=%v", data, err)
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("unconfirmed import modified destination: %v", err)
	}
}

func TestImportLegacyRefusesEmptySourceAndNeverMergesExistingDestination(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	emptySource := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(emptySource, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	emptyDestination := filepath.Join(dir, "empty-destination", "state.db")
	if _, err := ImportLegacyWorkspace(ctx, emptySource, emptyDestination, true); err == nil {
		t.Fatal("accepted an empty legacy source")
	}
	if _, err := os.Lstat(emptyDestination); !os.IsNotExist(err) {
		t.Fatalf("empty source created destination: %v", err)
	}

	source := filepath.Join(dir, "valid.db")
	legacy, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "existing", "state.db")
	if err = os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := []byte("existing isolated workspace must not be merged or replaced")
	if err = os.WriteFile(destination, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = ImportLegacyWorkspace(ctx, source, destination, true); err == nil || err.Error() != "IMPORT_DESTINATION_EXISTS" {
		t.Fatalf("existing destination error=%v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != string(marker) {
		t.Fatalf("destination changed during refused merge: %q err=%v", got, err)
	}
}

func TestImportLegacyRejectsSymlinkSourceWithoutFollowingIt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.db")
	state, err := Open(target)
	if err != nil {
		t.Fatal(err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.db")
	if err = os.Symlink(target, source); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	destination := filepath.Join(dir, "new", "state.db")
	if _, err = ImportLegacyWorkspace(ctx, source, destination, true); err == nil || err.Error() != "IMPORT_SOURCE_INVALID" {
		t.Fatalf("symlink import source error=%v", err)
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("symlink source changed destination: %v", err)
	}
}
