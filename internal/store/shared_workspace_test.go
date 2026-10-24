package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigration13BackfillsOnlyExactCanonicalProjectMatches(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-v12.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for version, migration := range []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12} {
		if _, err = db.Exec(migration); err != nil {
			t.Fatalf("apply migration %d: %v", version+1, err)
		}
		if _, err = db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`, version+1, "2026-10-24T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	matchedPath, unmatchedPath := t.TempDir(), t.TempDir()
	if _, err = db.Exec(`INSERT INTO projects(id,canonical_path,display_name,allowed_scopes_json,created_at) VALUES('matched',?,'matched','[]','2026-10-24T00:00:00Z')`, matchedPath); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO runs(id,project,prompt,state,created_at) VALUES('run-matched',?,'matched','COMPLETED','2026-10-24T00:00:00Z'),('run-unmatched',?,'unmatched','COMPLETED','2026-10-24T00:00:00Z')`, matchedPath, unmatchedPath); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	state, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var matchedID, unmatchedID sql.NullString
	if err = state.DB.QueryRow(`SELECT project_id FROM runs WHERE id='run-matched'`).Scan(&matchedID); err != nil {
		t.Fatal(err)
	}
	if err = state.DB.QueryRow(`SELECT project_id FROM runs WHERE id='run-unmatched'`).Scan(&unmatchedID); err != nil {
		t.Fatal(err)
	}
	if !matchedID.Valid || matchedID.String != "matched" || unmatchedID.Valid {
		t.Fatalf("backfill matched=%v unmatched=%v", matchedID, unmatchedID)
	}
	var metadataCount, importTableCount, indexCount int
	if err = state.DB.QueryRow(`SELECT COUNT(*) FROM workspace_metadata`).Scan(&metadataCount); err != nil {
		t.Fatal(err)
	}
	if err = state.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='workspace_imports'`).Scan(&importTableCount); err != nil {
		t.Fatal(err)
	}
	if err = state.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='runs_project_history'`).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if metadataCount != 1 || importTableCount != 1 || indexCount != 1 {
		t.Fatalf("migration artifacts metadata=%d imports=%d index=%d", metadataCount, importTableCount, indexCount)
	}
	var version int
	if err = state.DB.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != currentSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}

func TestSharedWorkspaceIdentityAndProjectRunAssociation(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	state, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	firstID, err := state.WorkspaceID(ctx)
	if err != nil || firstID == "" {
		t.Fatalf("workspace id=%q err=%v", firstID, err)
	}
	project := t.TempDir()
	if err = state.RegisterProject(ctx, project, "first", []string{project}); err != nil {
		t.Fatal(err)
	}
	registered, err := state.ProjectByPath(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if err = state.RegisterProject(ctx, project, "renamed", []string{"/different/scope"}); err != nil {
		t.Fatal(err)
	}
	repeated, err := state.ProjectByPath(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != registered.ID || repeated.DisplayName != "first" || len(repeated.AllowedScopes) != 1 || repeated.AllowedScopes[0] != project {
		t.Fatalf("idempotent registration overwrote canonical record: %+v", repeated)
	}
	linked, err := state.CreateRun(ctx, project, "linked run")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := state.CreateRun(ctx, filepath.Join(t.TempDir(), "unregistered"), "unassigned history")
	if err != nil {
		t.Fatal(err)
	}
	got, err := state.GetRun(ctx, linked.ID)
	if err != nil || got.ProjectID != registered.ID {
		t.Fatalf("run association=%q err=%v", got.ProjectID, err)
	}
	page, err := state.RunsPageForProject(ctx, registered.ID, 100, 0)
	if err != nil || len(page) != 1 || page[0].ID != linked.ID {
		t.Fatalf("filtered runs=%+v err=%v", page, err)
	}
	count, err := state.RunCountForProject(ctx, registered.ID)
	if err != nil || count != 1 {
		t.Fatalf("filtered count=%d err=%v", count, err)
	}
	all, err := state.Runs(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("legacy all-project history omitted: runs=%+v err=%v", all, err)
	}
	for _, run := range all {
		if run.ID == legacy.ID && run.ProjectID != "" {
			t.Fatalf("legacy run was assigned without an exact match: %+v", run)
		}
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	secondID, err := reopened.WorkspaceID(ctx)
	if err != nil || secondID != firstID {
		t.Fatalf("workspace identity was not stable: before=%q after=%q err=%v", firstID, secondID, err)
	}
}
