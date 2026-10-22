package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spectide/spectide/internal/model"
)

func TestBackupRestoreValidatesAndPreservesRuns(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateRun(ctx, "/project", "backup fixture")
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "state.backup.db")
	if err = s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backup)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions=%v err=%v", info, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(dir, "restored.db")
	if err = RestoreBackup(ctx, backup, restored); err != nil {
		t.Fatal(err)
	}
	copyStore, err := Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := copyStore.GetRun(ctx, run.ID); err != nil || got.Prompt != "backup fixture" {
		t.Fatalf("restored run=%+v err=%v", got, err)
	}
	copyStore.Close()
	if err = RestoreBackup(ctx, backup, restored); err == nil || !strings.Contains(err.Error(), "RESTORE_DESTINATION_EXISTS") {
		t.Fatalf("restore overwrote existing database: %v", err)
	}

	newer := filepath.Join(dir, "newer.db")
	backupBytes, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(newer, backupBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	unsupported, err := sql.Open("sqlite", newer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = unsupported.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(999,'2099-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	unsupported.Close()
	if err = RestoreBackup(ctx, newer, filepath.Join(dir, "bad-restore.db")); err == nil || !strings.Contains(err.Error(), "RESTORE_SCHEMA_UNSUPPORTED") {
		t.Fatalf("newer schema was accepted: %v", err)
	}
}

func TestRetentionExpiresOldDetailsAndPreservesActiveRuns(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	terminal, err := s.CreateRun(ctx, "/project", "terminal")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(ctx, terminal.ID, model.Blocked); err != nil {
		t.Fatal(err)
	}
	active, err := s.CreateRun(ctx, "/project", "active")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-detailRetention - time.Hour).Format(time.RFC3339Nano)
	for _, run := range []model.Run{terminal, active} {
		if _, err = s.DB.ExecContext(ctx, `UPDATE events SET timestamp=?,payload_json='{"detail":"private activity"}' WHERE run_id=?`, old, run.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.SaveCheckpoint(ctx, model.Checkpoint{RunID: run.ID, Stage: "plan", WorkspaceFingerprint: "fingerprint", StateJSON: `{"detail":"checkpoint"}`, CreatedAt: time.Now().UTC().Add(-detailRetention - time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.PruneRetention(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var terminalPayload, activePayload, terminalCheckpoint, activeCheckpoint string
	if err = s.DB.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE run_id=? ORDER BY seq DESC LIMIT 1`, terminal.ID).Scan(&terminalPayload); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE run_id=? ORDER BY seq DESC LIMIT 1`, active.ID).Scan(&activePayload); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT state_json FROM checkpoints WHERE run_id=?`, terminal.ID).Scan(&terminalCheckpoint); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT state_json FROM checkpoints WHERE run_id=?`, active.ID).Scan(&activeCheckpoint); err != nil {
		t.Fatal(err)
	}
	if terminalPayload != `{}` || terminalCheckpoint != `{}` {
		t.Fatalf("terminal detail not expired: event=%s checkpoint=%s", terminalPayload, terminalCheckpoint)
	}
	if activePayload == `{}` || activeCheckpoint == `{}` {
		t.Fatal("active run details were pruned")
	}
	if _, err = s.GetRun(ctx, active.ID); err != nil {
		t.Fatal("active summary was not preserved:", err)
	}
}

func TestDetailedHistoryCapPrunesOldestTerminalAndRejectsAllActive(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	for i := 0; i < maxDetailedRuns; i++ {
		state := model.Created
		ended := any(nil)
		if i == 0 {
			state = model.Completed
			ended = created
		}
		if _, err = s.DB.ExecContext(ctx, `INSERT INTO runs(id,project,prompt,state,created_at,started_at,ended_at) VALUES(?,?,?,?,?,?,?)`, fmt.Sprintf("run-%04d", i), "/project", "summary", state, created, nil, ended); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO events(run_id,seq,timestamp,stage,type,severity,payload_json) VALUES(?,1,?,'','run.completed','info','{"detail":"old"}')`, "run-0000", created); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO quality_gates(run_id,gate_key,verdict,evidence) VALUES(?,?,?,?)`, "run-0000", "required_tests", "PASS", "preserved summary verdict"); err != nil {
		t.Fatal(err)
	}
	newRun, err := s.CreateRun(ctx, "/project", "new run")
	if err != nil {
		t.Fatal(err)
	}
	var prunedAt, payload, verdict string
	if err = s.DB.QueryRowContext(ctx, `SELECT details_pruned_at FROM runs WHERE id='run-0000'`).Scan(&prunedAt); err != nil || prunedAt == "" {
		t.Fatalf("oldest terminal run was not pruned: %q err=%v", prunedAt, err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE run_id='run-0000'`).Scan(&payload); err != nil || payload != `{}` {
		t.Fatalf("old event detail=%s err=%v", payload, err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT verdict FROM quality_gates WHERE run_id='run-0000'`).Scan(&verdict); err != nil || verdict != "PASS" {
		t.Fatalf("preserved verdict=%s err=%v", verdict, err)
	}
	if _, err = s.GetRun(ctx, newRun.ID); err != nil {
		t.Fatal("new run missing after pruning:", err)
	}

	activeStore, err := Open(filepath.Join(dir, "active-cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer activeStore.Close()
	for i := 0; i < maxDetailedRuns; i++ {
		if _, err = activeStore.DB.ExecContext(ctx, `INSERT INTO runs(id,project,prompt,state,created_at) VALUES(?,?,?,?,?)`, fmt.Sprintf("active-%04d", i), "/project", "active", model.Running, created); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = activeStore.CreateRun(ctx, "/project", "must be rejected"); err == nil || !strings.Contains(err.Error(), "RETENTION_CAP_ACTIVE") {
		t.Fatalf("all-active detailed history was not rejected: %v", err)
	}
}
