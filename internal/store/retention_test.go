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

func TestRetentionExpiresExecutionDetailsAtTimestampBoundary(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "detail-boundary.db"))
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
	anchor := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	cutoff := anchor.Add(-detailRetention)
	for name, finished := range map[string]time.Time{"expired": cutoff.Add(-time.Nanosecond), "boundary": cutoff} {
		started := finished.Add(-time.Second)
		attempt, startErr := s.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: terminal.ID, Stage: name, Phase: name, Provider: "fake", Profile: "retention-test", StartedAt: started})
		if startErr != nil {
			t.Fatal(startErr)
		}
		attempt.Result, attempt.FinishedAt, attempt.Error = "FAILED", &finished, "sanitized detail summary"
		attempt.EvidenceJSON = `{"detail":"provider output"}`
		if err = s.CompleteExecutionAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		_, err = s.SaveQualityExecution(ctx, model.QualityExecution{RunID: terminal.ID, Phase: name, ProcessID: 1, StartedAt: started, FinishedAt: finished, Results: []model.QualityResult{{Key: "check_" + name, Verdict: "FAIL", Command: []string{"go", "test", "./internal/..."}, Summary: "check output detail", StartedAt: finished}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	activeAttempt, err := s.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: active.ID, Stage: "old", Phase: "old", Provider: "fake", Profile: "retention-test", StartedAt: cutoff.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	activeFinished := cutoff.Add(-time.Hour + time.Second)
	activeAttempt.Result, activeAttempt.FinishedAt, activeAttempt.EvidenceJSON = "SUCCEEDED", &activeFinished, `{"detail":"active"}`
	if err = s.CompleteExecutionAttempt(ctx, activeAttempt); err != nil {
		t.Fatal(err)
	}
	if err = s.PruneRetention(ctx, anchor); err != nil {
		t.Fatal(err)
	}
	attempts, err := s.ExecutionAttempts(ctx, terminal.ID)
	if err != nil {
		t.Fatal(err)
	}
	byStage := map[string]model.ExecutionAttempt{}
	for _, attempt := range attempts {
		byStage[attempt.Stage] = attempt
	}
	if byStage["expired"].EvidenceJSON != `{}` || byStage["expired"].Error != "" {
		t.Fatalf("expired attempt detail was retained: %+v", byStage["expired"])
	}
	if byStage["boundary"].EvidenceJSON == `{}` {
		t.Fatalf("exact 90-day boundary was expired: %+v", byStage["boundary"])
	}
	quality, err := s.QualityExecutions(ctx, terminal.ID)
	if err != nil {
		t.Fatal(err)
	}
	qualityByPhase := map[string]model.QualityExecution{}
	for _, execution := range quality {
		qualityByPhase[execution.Phase] = execution
	}
	if got := qualityByPhase["expired"].Results[0]; got.Summary != "" || len(got.Command) != 0 || got.Verdict != "FAIL" {
		t.Fatalf("expired quality detail/outcome=%+v", got)
	}
	if got := qualityByPhase["boundary"].Results[0]; got.Summary == "" || len(got.Command) == 0 || got.Verdict != "FAIL" {
		t.Fatalf("quality at exact 90-day boundary changed: %+v", got)
	}
	activeAttempts, err := s.ExecutionAttempts(ctx, active.ID)
	if err != nil || len(activeAttempts) != 1 || activeAttempts[0].EvidenceJSON == `{}` {
		t.Fatalf("active execution detail changed: %+v err=%v", activeAttempts, err)
	}
}

func TestRetentionMaintenanceRunsWhileDaemonIsIdle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := Open(filepath.Join(t.TempDir(), "idle-maintenance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run, err := s.CreateRun(context.Background(), "/project", "idle retention")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunState(context.Background(), run.ID, model.Blocked); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-detailRetention - time.Hour).Format(time.RFC3339Nano)
	if _, err = s.DB.Exec(`UPDATE events SET timestamp=?,payload_json='{"detail":"old"}' WHERE run_id=?`, old, run.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		s.RunRetentionMaintenance(ctx, 5*time.Millisecond, func(error) {})
		close(done)
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var payload string
		if err = s.DB.QueryRow(`SELECT payload_json FROM events WHERE run_id=? ORDER BY seq DESC LIMIT 1`, run.ID).Scan(&payload); err != nil {
			t.Fatal(err)
		}
		if payload == `{}` {
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("maintenance goroutine did not stop after cancellation")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("idle retention maintenance did not expire old details")
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
	attempt, err := s.StartExecutionAttempt(ctx, model.ExecutionAttempt{RunID: "run-0000", Stage: "implement", Phase: "implement", Provider: "fake", Profile: "retention-test"})
	if err != nil {
		t.Fatal(err)
	}
	finished := time.Now().UTC()
	attempt.Result, attempt.FinishedAt, attempt.EvidenceJSON = "SUCCEEDED", &finished, `{"detail":"provider output"}`
	if err = s.CompleteExecutionAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveQualityExecution(ctx, model.QualityExecution{RunID: "run-0000", Phase: "tests", ProcessID: 1, Results: []model.QualityResult{{Key: "unit_tests", Verdict: "PASS", Command: []string{"go", "test", "./..."}, Summary: "all detailed output", StartedAt: finished}}}); err != nil {
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
	attempts, err := s.ExecutionAttempts(ctx, "run-0000")
	if err != nil || len(attempts) != 1 || attempts[0].Result != "SUCCEEDED" || attempts[0].EvidenceJSON != `{}` {
		t.Fatalf("attempt summary/detail after cap prune=%+v err=%v", attempts, err)
	}
	quality, err := s.QualityExecutions(ctx, "run-0000")
	if err != nil || len(quality) != 1 || quality[0].Verdict != "PASS" || len(quality[0].Results) != 1 || quality[0].Results[0].Verdict != "PASS" || quality[0].Results[0].Summary != "" || len(quality[0].Results[0].Command) != 0 {
		t.Fatalf("quality compact outcome/detail after cap prune=%+v err=%v", quality, err)
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
