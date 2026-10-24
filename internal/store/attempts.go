package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spectide/spectide/internal/events"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/security"
)

func nextEventSequence(ctx context.Context, tx *sql.Tx, runID string) (int64, error) {
	var sequence int64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM events WHERE run_id=?`, runID).Scan(&sequence)
	return sequence, err
}

func (s *Store) StartExecutionAttempt(ctx context.Context, attempt model.ExecutionAttempt) (model.ExecutionAttempt, error) {
	if attempt.RunID == "" || attempt.Stage == "" || attempt.Phase == "" || attempt.Provider == "" || attempt.Profile == "" {
		return attempt, fmt.Errorf("EXECUTION_ATTEMPT_FIELDS_REQUIRED")
	}
	for _, identity := range []string{attempt.Stage, attempt.Phase, attempt.Provider, attempt.Profile, attempt.Model, attempt.Effort} {
		if security.Redact(identity) != identity {
			return attempt, fmt.Errorf("EXECUTION_ATTEMPT_SECRET_IDENTITY_REJECTED")
		}
	}
	if attempt.ID == "" {
		attempt.ID = uuid.NewString()
	}
	if attempt.StartedAt.IsZero() {
		attempt.StartedAt = now()
	}
	attempt.Result = "RUNNING"
	attempt.EvidenceJSON = "{}"
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return attempt, err
	}
	defer tx.Rollback()
	if attempt.Iteration == 0 {
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(iteration),0)+1 FROM execution_attempts WHERE run_id=? AND stage=? AND phase=?`, attempt.RunID, attempt.Stage, attempt.Phase).Scan(&attempt.Iteration); err != nil {
			return attempt, err
		}
	}
	attempt.StartEventSeq, err = nextEventSequence(ctx, tx, attempt.RunID)
	if err != nil {
		return attempt, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO execution_attempts(id,run_id,stage,phase,iteration,provider,profile,model,effort,started_at,start_event_seq,result,evidence_json,artifact_ids_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, attempt.ID, attempt.RunID, attempt.Stage, attempt.Phase, attempt.Iteration, attempt.Provider, attempt.Profile, attempt.Model, attempt.Effort, attempt.StartedAt.Format(time.RFC3339Nano), attempt.StartEventSeq, attempt.Result, `{}`, `[]`)
	if err != nil {
		return attempt, err
	}
	if err = appendEvent(ctx, tx, attempt.RunID, attempt.Stage, "execution.attempt_started", "info", map[string]any{"attempt_id": attempt.ID, "phase": attempt.Phase, "iteration": attempt.Iteration, "provider": attempt.Provider, "profile": attempt.Profile, "model": attempt.Model, "effort": attempt.Effort}); err != nil {
		return attempt, err
	}
	return attempt, tx.Commit()
}

func (s *Store) CompleteExecutionAttempt(ctx context.Context, attempt model.ExecutionAttempt) error {
	if attempt.ID == "" {
		return fmt.Errorf("EXECUTION_ATTEMPT_ID_REQUIRED")
	}
	switch attempt.Result {
	case "SUCCEEDED", "FAILED", "CANCELLED":
	default:
		return fmt.Errorf("EXECUTION_ATTEMPT_RESULT_INVALID")
	}
	if attempt.FinishedAt == nil {
		finished := now()
		attempt.FinishedAt = &finished
	}
	if attempt.DurationMillis < 0 || attempt.ProcessID < 0 || (attempt.ExitCode < 0 && attempt.Result == "SUCCEEDED") || attempt.FinishedAt.Before(attempt.StartedAt) {
		return fmt.Errorf("EXECUTION_ATTEMPT_RUNTIME_INVALID")
	}
	attempt.Error = security.Redact(attempt.Error)
	if attempt.EvidenceJSON == "" {
		attempt.EvidenceJSON = `{}`
	}
	var evidence any
	if err := json.Unmarshal([]byte(attempt.EvidenceJSON), &evidence); err != nil {
		return fmt.Errorf("EXECUTION_ATTEMPT_EVIDENCE_INVALID")
	}
	evidenceJSON, err := json.Marshal(events.SanitizePayload(evidence))
	if err != nil {
		return err
	}
	attempt.EvidenceJSON = security.Redact(string(evidenceJSON))
	artifactsJSON, err := json.Marshal(attempt.ArtifactIDs)
	if err != nil {
		return err
	}
	for _, value := range []string{attempt.StdoutSHA256, attempt.StderrSHA256} {
		if value != "" && (len(value) != sha256.Size*2 || !isLowerHex(value)) {
			return fmt.Errorf("EXECUTION_ATTEMPT_OUTPUT_HASH_INVALID")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var runID, stage, phase string
	var iteration int
	if err = tx.QueryRowContext(ctx, `SELECT run_id,stage,phase,iteration FROM execution_attempts WHERE id=? AND result='RUNNING'`, attempt.ID).Scan(&runID, &stage, &phase, &iteration); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("EXECUTION_ATTEMPT_NOT_RUNNING")
		}
		return err
	}
	attempt.CompletionEventSeq, err = nextEventSequence(ctx, tx, runID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE execution_attempts SET finished_at=?,completion_event_seq=?,process_id=?,duration_ms=?,exit_code=?,result=?,error=?,stdout_sha256=?,stderr_sha256=?,evidence_json=?,artifact_ids_json=? WHERE id=? AND result='RUNNING'`, attempt.FinishedAt.Format(time.RFC3339Nano), attempt.CompletionEventSeq, attempt.ProcessID, attempt.DurationMillis, attempt.ExitCode, attempt.Result, attempt.Error, attempt.StdoutSHA256, attempt.StderrSHA256, attempt.EvidenceJSON, string(artifactsJSON), attempt.ID)
	if err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, runID, stage, "execution.attempt_completed", "info", map[string]any{"attempt_id": attempt.ID, "phase": phase, "iteration": iteration, "result": attempt.Result, "process_id": attempt.ProcessID, "duration_ms": attempt.DurationMillis, "exit_code": attempt.ExitCode, "error": attempt.Error, "stdout_sha256": attempt.StdoutSHA256, "stderr_sha256": attempt.StderrSHA256, "artifact_ids": attempt.ArtifactIDs}); err != nil {
		return err
	}
	return tx.Commit()
}

// InterruptRunningAttempts finalizes provider invocations left open by an
// owner-process exit. Their result is ambiguous, so they are recorded as
// FAILED with an explicit interruption reason and never rewritten later.
func (s *Store) InterruptRunningAttempts(ctx context.Context, runID, reason string) error {
	if runID == "" {
		return fmt.Errorf("RUN_ID_REQUIRED")
	}
	reason = security.Redact(reason)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = interruptRunningAttemptsTx(ctx, tx, runID, reason); err != nil {
		return err
	}
	return tx.Commit()
}

func interruptRunningAttemptsTx(ctx context.Context, tx *sql.Tx, runID, reason string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,stage,phase,iteration,started_at FROM execution_attempts WHERE run_id=? AND result='RUNNING' ORDER BY started_at,id`, runID)
	if err != nil {
		return err
	}
	type interrupted struct {
		id, stage, phase, started string
		iteration                 int
	}
	var pending []interrupted
	for rows.Next() {
		var item interrupted
		if err = rows.Scan(&item.id, &item.stage, &item.phase, &item.iteration, &item.started); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, item := range pending {
		finished := now()
		sequence, seqErr := nextEventSequence(ctx, tx, runID)
		if seqErr != nil {
			return seqErr
		}
		if _, err = tx.ExecContext(ctx, `UPDATE execution_attempts SET finished_at=?,completion_event_seq=?,result='FAILED',error=? WHERE id=? AND result='RUNNING'`, finished.Format(time.RFC3339Nano), sequence, reason, item.id); err != nil {
			return err
		}
		if err = appendEvent(ctx, tx, runID, item.stage, "execution.attempt_interrupted", "error", map[string]any{"attempt_id": item.id, "phase": item.phase, "iteration": item.iteration, "result": "FAILED", "reason": reason}); err != nil {
			return err
		}
	}
	return nil
}

func isLowerHex(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func (s *Store) ExecutionAttempts(ctx context.Context, runID string) ([]model.ExecutionAttempt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,run_id,stage,phase,iteration,provider,profile,model,effort,started_at,COALESCE(finished_at,''),start_event_seq,completion_event_seq,process_id,duration_ms,exit_code,result,error,stdout_sha256,stderr_sha256,evidence_json,artifact_ids_json FROM execution_attempts WHERE run_id=? ORDER BY started_at,id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.ExecutionAttempt, 0)
	for rows.Next() {
		var attempt model.ExecutionAttempt
		var started, finished, artifacts string
		if err = rows.Scan(&attempt.ID, &attempt.RunID, &attempt.Stage, &attempt.Phase, &attempt.Iteration, &attempt.Provider, &attempt.Profile, &attempt.Model, &attempt.Effort, &started, &finished, &attempt.StartEventSeq, &attempt.CompletionEventSeq, &attempt.ProcessID, &attempt.DurationMillis, &attempt.ExitCode, &attempt.Result, &attempt.Error, &attempt.StdoutSHA256, &attempt.StderrSHA256, &attempt.EvidenceJSON, &artifacts); err != nil {
			return nil, err
		}
		attempt.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		attempt.FinishedAt = parse(finished)
		if err = json.Unmarshal([]byte(artifacts), &attempt.ArtifactIDs); err != nil {
			return nil, fmt.Errorf("EXECUTION_ATTEMPT_ARTIFACT_LINKS_INVALID")
		}
		out = append(out, attempt)
	}
	return out, rows.Err()
}

func (s *Store) SaveQualityExecution(ctx context.Context, execution model.QualityExecution) (model.QualityExecution, error) {
	if execution.RunID == "" || execution.Phase == "" || len(execution.Results) == 0 {
		return execution, fmt.Errorf("QUALITY_EXECUTION_FIELDS_REQUIRED")
	}
	if execution.ID == "" {
		execution.ID = uuid.NewString()
	}
	if execution.ProcessID < 1 {
		return execution, fmt.Errorf("QUALITY_EXECUTION_PROCESS_REQUIRED")
	}
	if execution.StartedAt.IsZero() {
		execution.StartedAt = now()
	}
	if execution.FinishedAt.IsZero() {
		execution.FinishedAt = now()
	}
	passed, blocked := true, false
	seen := make(map[string]struct{}, len(execution.Results))
	for index := range execution.Results {
		result := &execution.Results[index]
		if result.Key == "" || security.Redact(result.Key) != result.Key {
			return execution, fmt.Errorf("QUALITY_RESULT_KEY_INVALID")
		}
		if _, ok := seen[result.Key]; ok {
			return execution, fmt.Errorf("QUALITY_RESULT_DUPLICATE")
		}
		seen[result.Key] = struct{}{}
		result.Summary = security.Redact(result.Summary)
		for i, value := range result.Command {
			result.Command[i] = security.Redact(value)
		}
		switch result.Verdict {
		case "PASS":
		case "FAIL":
			passed = false
		case "BLOCKED", "NOT_RUN":
			passed, blocked = false, true
		default:
			return execution, fmt.Errorf("QUALITY_RESULT_VERDICT_INVALID")
		}
	}
	execution.Verdict = "PASS"
	if blocked {
		execution.Verdict = "BLOCKED"
	} else if !passed {
		execution.Verdict = "FAIL"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return execution, err
	}
	defer tx.Rollback()
	if execution.Iteration == 0 {
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(iteration),0)+1 FROM quality_executions WHERE run_id=? AND phase=?`, execution.RunID, execution.Phase).Scan(&execution.Iteration); err != nil {
			return execution, err
		}
	}
	execution.CompletionEventSeq, err = nextEventSequence(ctx, tx, execution.RunID)
	if err != nil {
		return execution, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO quality_executions(id,run_id,phase,iteration,process_id,started_at,finished_at,completion_event_seq,verdict) VALUES(?,?,?,?,?,?,?,?,?)`, execution.ID, execution.RunID, execution.Phase, execution.Iteration, execution.ProcessID, execution.StartedAt.Format(time.RFC3339Nano), execution.FinishedAt.Format(time.RFC3339Nano), execution.CompletionEventSeq, execution.Verdict)
	if err != nil {
		return execution, err
	}
	for _, result := range execution.Results {
		commandJSON, marshalErr := json.Marshal(result.Command)
		if marshalErr != nil {
			return execution, marshalErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO quality_results(execution_id,run_id,check_key,verdict,command_json,process_id,exit_code,duration_ns,summary,started_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, execution.ID, execution.RunID, result.Key, result.Verdict, string(commandJSON), result.ProcessID, result.ExitCode, result.Duration.Nanoseconds(), result.Summary, result.StartedAt.Format(time.RFC3339Nano))
		if err != nil {
			return execution, err
		}
	}
	if err = appendEvent(ctx, tx, execution.RunID, execution.Phase, "quality.execution_completed", "info", map[string]any{"execution_id": execution.ID, "phase": execution.Phase, "iteration": execution.Iteration, "verdict": execution.Verdict, "check_count": len(execution.Results)}); err != nil {
		return execution, err
	}
	return execution, tx.Commit()
}

func (s *Store) QualityExecutions(ctx context.Context, runID string) ([]model.QualityExecution, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,run_id,phase,iteration,process_id,started_at,finished_at,completion_event_seq,verdict FROM quality_executions WHERE run_id=? ORDER BY started_at,id`, runID)
	if err != nil {
		return nil, err
	}
	out := make([]model.QualityExecution, 0)
	for rows.Next() {
		var execution model.QualityExecution
		var started, finished string
		if err = rows.Scan(&execution.ID, &execution.RunID, &execution.Phase, &execution.Iteration, &execution.ProcessID, &started, &finished, &execution.CompletionEventSeq, &execution.Verdict); err != nil {
			return nil, err
		}
		execution.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		execution.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
		out = append(out, execution)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for index := range out {
		resultRows, queryErr := s.DB.QueryContext(ctx, `SELECT check_key,verdict,command_json,process_id,exit_code,duration_ns,summary,started_at FROM quality_results WHERE execution_id=? ORDER BY check_key`, out[index].ID)
		if queryErr != nil {
			return nil, queryErr
		}
		for resultRows.Next() {
			var result model.QualityResult
			var commandJSON, resultStarted string
			var duration int64
			if queryErr = resultRows.Scan(&result.Key, &result.Verdict, &commandJSON, &result.ProcessID, &result.ExitCode, &duration, &result.Summary, &resultStarted); queryErr != nil {
				resultRows.Close()
				return nil, queryErr
			}
			if queryErr = json.Unmarshal([]byte(commandJSON), &result.Command); queryErr != nil {
				resultRows.Close()
				return nil, fmt.Errorf("QUALITY_RESULT_COMMAND_INVALID")
			}
			result.Duration = time.Duration(duration)
			result.StartedAt, _ = time.Parse(time.RFC3339Nano, resultStarted)
			out[index].Results = append(out[index].Results, result)
		}
		if queryErr = resultRows.Err(); queryErr != nil {
			resultRows.Close()
			return nil, queryErr
		}
		if queryErr = resultRows.Close(); queryErr != nil {
			return nil, queryErr
		}
	}
	return out, nil
}
