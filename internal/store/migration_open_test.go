package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func openRawSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func TestOpenRejectsForwardIncompatibleMigrationVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	db := openRawSQLite(t, path)
	if _, err := db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`, currentSchemaVersion+1, "future"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(path); err == nil || !strings.Contains(err.Error(), "STORE_SCHEMA_VERSION_UNSUPPORTED") {
		if opened != nil {
			_ = opened.Close()
		}
		t.Fatalf("forward-incompatible database was accepted: %v", err)
	}
	verify, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer verify.Close()
	var version int
	if err = verify.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != currentSchemaVersion+1 {
		t.Fatalf("future schema metadata changed: version=%d err=%v", version, err)
	}
	var tables int
	if err = verify.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name!='schema_migrations'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("unsupported schema was mutated: tables=%d err=%v", tables, err)
	}
}

func TestOpenRejectsInconsistentMigrationHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gap.db")
	db := openRawSQLite(t, path)
	if _, err := db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(1,'one'),(3,'three')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(path); err == nil || !strings.Contains(err.Error(), "STORE_MIGRATION_METADATA_INVALID") {
		if opened != nil {
			_ = opened.Close()
		}
		t.Fatalf("migration version gap was accepted: %v", err)
	}
}

func TestOpenMigrationFailureClosesConnectionAndRollsBackMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration-failure.db")
	db := openRawSQLite(t, path)
	if _, err := db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`, currentSchemaVersion-1, "before-failure"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(path); err == nil || !strings.Contains(err.Error(), "STORE_MIGRATION_FAILED") {
		if opened != nil {
			_ = opened.Close()
		}
		t.Fatalf("deliberately invalid migration state unexpectedly opened: %v", err)
	}
	verify, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer verify.Close()
	var version int
	if err = verify.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != currentSchemaVersion-1 {
		t.Fatalf("failed migration altered history: version=%d err=%v", version, err)
	}
	var qualityGates int
	if err = verify.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='quality_gates'`).Scan(&qualityGates); err != nil || qualityGates != 0 {
		t.Fatalf("failed migration left partial schema: table count=%d err=%v", qualityGates, err)
	}
}
