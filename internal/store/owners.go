package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/spectide/spectide/internal/model"
	workspaceguard "github.com/spectide/spectide/internal/workspace"
)

const RunOwnerLease = 3 * time.Second

type runOwnerState struct {
	id        string
	project   string
	state     model.RunState
	heartbeat string
}

func (s *Store) HeartbeatRunOwner(ctx context.Context, runID, ownerID string, at time.Time) error {
	if at.IsZero() {
		at = now()
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE run_owners SET heartbeat_at=? WHERE run_id=? AND owner_id=?`, at.Format(time.RFC3339Nano), runID, ownerID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("RUN_OWNER_NOT_REGISTERED")
	}
	return nil
}

func (s *Store) RunOwnerFresh(ctx context.Context, runID string, at time.Time) (bool, error) {
	var heartbeat string
	err := s.DB.QueryRowContext(ctx, `SELECT heartbeat_at FROM run_owners WHERE run_id=?`, runID).Scan(&heartbeat)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	updated, err := time.Parse(time.RFC3339Nano, heartbeat)
	if err != nil {
		return false, fmt.Errorf("RUN_OWNER_HEARTBEAT_INVALID")
	}
	if at.IsZero() {
		at = now()
	}
	return !updated.Before(at.Add(-RunOwnerLease)), nil
}

// VerifyRunWorkspaceLock confirms the run still exclusively owns its canonical
// workspace path before any resume signal or restart claim is issued.
func (s *Store) VerifyRunWorkspaceLock(ctx context.Context, canonicalPath, runID string) error {
	var lockedRun string
	if err := s.DB.QueryRowContext(ctx, `SELECT run_id FROM workspace_locks WHERE canonical_path=?`, canonicalPath).Scan(&lockedRun); err != nil {
		return fmt.Errorf("WORKSPACE_DRIFT: exclusive workspace lock is missing")
	}
	if lockedRun != runID {
		return fmt.Errorf("WORKSPACE_DRIFT: exclusive workspace lock belongs to another run")
	}
	var otherLocks int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_locks WHERE run_id=?`, runID).Scan(&otherLocks); err != nil {
		return err
	}
	if otherLocks != 1 {
		return fmt.Errorf("WORKSPACE_DRIFT: run does not own exactly one workspace lock")
	}
	return nil
}

// ClaimExpiredPausedRun transfers a paused run to a new process only when its
// previous lease is stale and the workspace lock still names this run. The
// caller must verify the immutable configuration and workspace checkpoint
// before claiming.
func (s *Store) ClaimExpiredPausedRun(ctx context.Context, runID, canonicalPath, ownerID string, at time.Time) error {
	if at.IsZero() {
		at = now()
	}
	if runID == "" || canonicalPath == "" || ownerID == "" {
		return fmt.Errorf("RUN_RECOVERY_FIELDS_REQUIRED")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state model.RunState
	if err = tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE id=?`, runID).Scan(&state); err != nil {
		return err
	}
	if state != model.Paused {
		return fmt.Errorf("RUN_CONTROL_NOT_ALLOWED: restart resume for %s", state)
	}
	var control string
	controlErr := tx.QueryRowContext(ctx, `SELECT action FROM run_controls WHERE run_id=?`, runID).Scan(&control)
	if controlErr == nil && control == "cancel" {
		return fmt.Errorf("RUN_CANCEL_PENDING")
	}
	if controlErr != nil && controlErr != sql.ErrNoRows {
		return controlErr
	}
	var lockedRun string
	if err = tx.QueryRowContext(ctx, `SELECT run_id FROM workspace_locks WHERE canonical_path=?`, canonicalPath).Scan(&lockedRun); err != nil {
		return fmt.Errorf("WORKSPACE_DRIFT: exclusive workspace lock is missing")
	}
	if lockedRun != runID {
		return fmt.Errorf("WORKSPACE_DRIFT: exclusive workspace lock belongs to another run")
	}
	var heartbeat string
	err = tx.QueryRowContext(ctx, `SELECT heartbeat_at FROM run_owners WHERE run_id=?`, runID).Scan(&heartbeat)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		updated, parseErr := time.Parse(time.RFC3339Nano, heartbeat)
		if parseErr != nil {
			return fmt.Errorf("RUN_OWNER_HEARTBEAT_INVALID")
		}
		if !updated.Before(at.Add(-RunOwnerLease)) {
			return fmt.Errorf("RUN_OWNER_STILL_ACTIVE")
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO run_owners(run_id,owner_id,heartbeat_at) VALUES(?,?,?) ON CONFLICT(run_id) DO UPDATE SET owner_id=excluded.owner_id,heartbeat_at=excluded.heartbeat_at`, runID, ownerID, at.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, runID, "", "run.recovery_claimed", "warning", map[string]any{"workspace": canonicalPath, "owner_id": ownerID, "checkpoint_verified": true}); err != nil {
		return err
	}
	return tx.Commit()
}

// BlockPausedRunForWorkspaceDrift closes a paused run after verification fails
// while preserving its immutable checkpoints, artifacts, and workspace files.
func (s *Store) BlockPausedRunForWorkspaceDrift(ctx context.Context, runID, reason string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE runs SET state=?,ended_at=? WHERE id=? AND state=?`, model.Blocked, now().Format(time.RFC3339Nano), runID, model.Paused)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("RUN_CONTROL_NOT_ALLOWED: drift block requires a paused run")
	}
	if err = finalizeTerminalGatesTx(ctx, tx, runID, model.Blocked); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, runID, "", "run.interrupted", "warning", map[string]any{"reason": "WORKSPACE_DRIFT: " + reason}); err != nil {
		return err
	}
	for _, table := range []string{"workspace_locks", "run_owners", "run_controls"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE run_id=?`, runID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecoverAbandonedRuns blocks running or paused work only after its persisted
// owner lease expires. Explicit resume verifies the checkpoint and workspace
// before taking over; callers use this method when reconciling abandoned work
// before starting a different run.
func (s *Store) RecoverAbandonedRuns(ctx context.Context, at time.Time) error {
	if at.IsZero() {
		at = now()
	}
	cutoff := at.Add(-RunOwnerLease)
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.state,r.project,COALESCE(o.heartbeat_at,''),r.created_at FROM runs r LEFT JOIN run_owners o ON o.run_id=r.id WHERE r.state IN (?,?,?)`, model.Created, model.Running, model.Paused)
	if err != nil {
		return err
	}
	var stale []runOwnerState
	for rows.Next() {
		var id, project, heartbeat, created string
		var state model.RunState
		if err = rows.Scan(&id, &state, &project, &heartbeat, &created); err != nil {
			rows.Close()
			return err
		}
		if heartbeat == "" && state == model.Created {
			heartbeat = created
		}
		updated, parseErr := time.Parse(time.RFC3339Nano, heartbeat)
		if heartbeat == "" || parseErr != nil || updated.Before(cutoff) {
			stale = append(stale, runOwnerState{id: id, project: project, state: state, heartbeat: heartbeat})
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range stale {
		if item.state == model.Created {
			if recoveryErr := s.recoverAbandonedCreatedRun(ctx, item, cutoff, at); recoveryErr != nil {
				return recoveryErr
			}
			continue
		}
		if item.state == model.Paused {
			control, controlErr := s.RunControl(ctx, item.id)
			if controlErr != nil {
				return controlErr
			}
			if control == "cancel" {
				if recoveryErr := s.cancelAbandonedPausedRun(ctx, item, cutoff, at); recoveryErr != nil {
					return recoveryErr
				}
			}
			// Paused work has a verified continuation checkpoint. Keep its
			// durable workspace reservation so explicit resume can claim it.
			continue
		}
		held, lockErr := workspaceguard.ProcessLockHeld(item.project)
		if lockErr != nil {
			return lockErr
		}
		if held {
			// A paused or stalled owner still has a live process/provider child.
			// Heartbeat expiry alone is not sufficient to reassign its writer.
			continue
		}
		tx, txErr := s.DB.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		var latestHeartbeat string
		ownerErr := tx.QueryRowContext(ctx, `SELECT heartbeat_at FROM run_owners WHERE run_id=?`, item.id).Scan(&latestHeartbeat)
		if ownerErr == nil {
			latest, parseErr := time.Parse(time.RFC3339Nano, latestHeartbeat)
			if parseErr == nil && !latest.Before(cutoff) {
				tx.Rollback()
				continue
			}
		} else if ownerErr != sql.ErrNoRows {
			tx.Rollback()
			return ownerErr
		}
		reason := "owner process exited without a verified resumable checkpoint; active invocation outcome is ambiguous"
		if txErr = interruptRunningAttemptsTx(ctx, tx, item.id, reason); txErr == nil {
			txErr = interruptActiveStagesTx(ctx, tx, item.id, reason)
		}
		if txErr != nil {
			tx.Rollback()
			return txErr
		}
		result, txErr := tx.ExecContext(ctx, `UPDATE runs SET state=?,ended_at=? WHERE id=? AND state=?`, model.Blocked, at.Format(time.RFC3339Nano), item.id, item.state)
		if txErr == nil {
			var changed int64
			changed, txErr = result.RowsAffected()
			if txErr == nil && changed == 1 {
				txErr = finalizeTerminalGatesTx(ctx, tx, item.id, model.Blocked)
				if txErr == nil {
					txErr = appendEvent(ctx, tx, item.id, "", "run.interrupted", "warning", map[string]any{"reason": "WORKSPACE_DRIFT: owner lease expired without verified checkpoint, artifact, and Git fingerprint recovery"})
				}
			}
		}
		if txErr == nil {
			_, txErr = tx.ExecContext(ctx, `DELETE FROM workspace_locks WHERE run_id=?`, item.id)
		}
		if txErr == nil {
			_, txErr = tx.ExecContext(ctx, `DELETE FROM run_owners WHERE run_id=?`, item.id)
		}
		if txErr != nil {
			tx.Rollback()
			return txErr
		}
		if txErr = tx.Commit(); txErr != nil {
			return txErr
		}
	}
	// A worker may have stopped and terminalized its run after its owner
	// identity was replaced. Keep that lock until its lease is stale and no
	// process lock remains, then remove only the orphaned terminal owner rows.
	terminalRows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.project,o.heartbeat_at FROM runs r JOIN run_owners o ON o.run_id=r.id WHERE r.state IN (?,?,?,?)`, model.Completed, model.Blocked, model.Failed, model.Cancelled)
	if err != nil {
		return err
	}
	type terminalOwner struct{ id, project, heartbeat string }
	var expired []terminalOwner
	for terminalRows.Next() {
		var item terminalOwner
		if err = terminalRows.Scan(&item.id, &item.project, &item.heartbeat); err != nil {
			terminalRows.Close()
			return err
		}
		updated, parseErr := time.Parse(time.RFC3339Nano, item.heartbeat)
		if parseErr != nil || updated.Before(cutoff) {
			expired = append(expired, item)
		}
	}
	if err = terminalRows.Err(); err != nil {
		terminalRows.Close()
		return err
	}
	if err = terminalRows.Close(); err != nil {
		return err
	}
	for _, item := range expired {
		held, lockErr := workspaceguard.ProcessLockHeld(item.project)
		if lockErr != nil {
			return lockErr
		}
		if held {
			continue
		}
		tx, txErr := s.DB.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		var latestHeartbeat string
		if txErr = tx.QueryRowContext(ctx, `SELECT heartbeat_at FROM run_owners WHERE run_id=?`, item.id).Scan(&latestHeartbeat); txErr == nil {
			latest, parseErr := time.Parse(time.RFC3339Nano, latestHeartbeat)
			if parseErr == nil && !latest.Before(cutoff) {
				tx.Rollback()
				continue
			}
		} else if txErr == sql.ErrNoRows {
			tx.Rollback()
			continue
		} else {
			tx.Rollback()
			return txErr
		}
		if _, txErr = tx.ExecContext(ctx, `DELETE FROM workspace_locks WHERE run_id=?`, item.id); txErr == nil {
			_, txErr = tx.ExecContext(ctx, `DELETE FROM run_owners WHERE run_id=?`, item.id)
		}
		if txErr != nil {
			tx.Rollback()
			return txErr
		}
		if txErr = tx.Commit(); txErr != nil {
			return txErr
		}
	}
	return nil
}

func (s *Store) recoverAbandonedCreatedRun(ctx context.Context, item runOwnerState, cutoff, at time.Time) error {
	processLock, err := workspaceguard.AcquireProcessLock(item.project)
	if err != nil {
		if strings.Contains(err.Error(), "WORKSPACE_LOCKED") {
			return nil
		}
		return err
	}
	defer processLock.Close()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state model.RunState
	if err = tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE id=?`, item.id).Scan(&state); err != nil {
		return err
	}
	if state != model.Created {
		return nil
	}
	var heartbeat string
	err = tx.QueryRowContext(ctx, `SELECT heartbeat_at FROM run_owners WHERE run_id=?`, item.id).Scan(&heartbeat)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `SELECT created_at FROM runs WHERE id=?`, item.id).Scan(&heartbeat)
	} else if err != nil {
		return err
	}
	if updated, parseErr := time.Parse(time.RFC3339Nano, heartbeat); parseErr == nil && !updated.Before(cutoff) {
		return nil
	}
	var action string
	controlErr := tx.QueryRowContext(ctx, `SELECT action FROM run_controls WHERE run_id=?`, item.id).Scan(&action)
	if controlErr != nil && controlErr != sql.ErrNoRows {
		return controlErr
	}
	terminal := model.Blocked
	eventType := "run.interrupted"
	reason := "launch owner exited before workflow execution; outcome is ambiguous and the original run was blocked without replay"
	severity := "warning"
	if action == "cancel" {
		terminal, eventType = model.Cancelled, "run.cancelled"
		reason = "cancellation requested; launch owner exited before workflow execution"
	}
	if err = interruptRunningAttemptsTx(ctx, tx, item.id, reason); err != nil {
		return err
	}
	if err = interruptActiveStagesTx(ctx, tx, item.id, reason); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runs SET state=?,ended_at=? WHERE id=? AND state=?`, terminal, at.Format(time.RFC3339Nano), item.id, model.Created)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return fmt.Errorf("RUN_RECOVERY_CONFLICT: created run changed concurrently")
	}
	if err = finalizeTerminalGatesTx(ctx, tx, item.id, terminal); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, item.id, "", eventType, severity, map[string]any{"reason": reason, "recovered": true}); err != nil {
		return err
	}
	for _, table := range []string{"workspace_locks", "run_owners", "run_controls"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE run_id=?`, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) cancelAbandonedPausedRun(ctx context.Context, item runOwnerState, cutoff, at time.Time) error {
	processLock, err := workspaceguard.AcquireProcessLock(item.project)
	if err != nil {
		if strings.Contains(err.Error(), "WORKSPACE_LOCKED") {
			return nil
		}
		return err
	}
	defer processLock.Close()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state model.RunState
	if err = tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE id=?`, item.id).Scan(&state); err != nil {
		return err
	}
	if state != model.Paused {
		return nil
	}
	var action, heartbeat string
	if err = tx.QueryRowContext(ctx, `SELECT action FROM run_controls WHERE run_id=?`, item.id).Scan(&action); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if action != "cancel" {
		return nil
	}
	err = tx.QueryRowContext(ctx, `SELECT heartbeat_at FROM run_owners WHERE run_id=?`, item.id).Scan(&heartbeat)
	if err == nil {
		updated, parseErr := time.Parse(time.RFC3339Nano, heartbeat)
		if parseErr == nil && !updated.Before(cutoff) {
			return nil
		}
	} else if err != sql.ErrNoRows {
		return err
	}
	reason := "cancellation requested; owner exited and no provider or quality child holds the workspace process fence"
	if err = interruptRunningAttemptsTx(ctx, tx, item.id, reason); err != nil {
		return err
	}
	if err = interruptActiveStagesTx(ctx, tx, item.id, reason); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runs SET state=?,ended_at=? WHERE id=? AND state=?`, model.Cancelled, at.Format(time.RFC3339Nano), item.id, model.Paused)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return fmt.Errorf("RUN_CONTROL_NOT_ALLOWED: abandoned cancellation changed concurrently")
	}
	if err = finalizeTerminalGatesTx(ctx, tx, item.id, model.Cancelled); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, item.id, "", "run.cancelled", "warning", map[string]any{"reason": reason, "recovered": true}); err != nil {
		return err
	}
	for _, table := range []string{"workspace_locks", "run_owners", "run_controls"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE run_id=?`, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FinalizeClaimedPausedCancellation completes a durable cancellation after a
// resumer has claimed the paused owner lease and holds the process fence.
func (s *Store) FinalizeClaimedPausedCancellation(ctx context.Context, runID, ownerID string, at time.Time) error {
	if at.IsZero() {
		at = now()
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state model.RunState
	if err = tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE id=?`, runID).Scan(&state); err != nil {
		return err
	}
	if state != model.Paused {
		return fmt.Errorf("RUN_CANCEL_NOT_PAUSED")
	}
	var action, currentOwner string
	if err = tx.QueryRowContext(ctx, `SELECT action FROM run_controls WHERE run_id=?`, runID).Scan(&action); err != nil {
		return err
	}
	if action != "cancel" {
		return fmt.Errorf("RUN_CANCEL_NOT_PENDING")
	}
	if err = tx.QueryRowContext(ctx, `SELECT owner_id FROM run_owners WHERE run_id=?`, runID).Scan(&currentOwner); err != nil {
		return err
	}
	if currentOwner != ownerID {
		return fmt.Errorf("RUN_OWNER_NOT_REGISTERED")
	}
	reason := "cancellation requested while paused; claimed owner stopped before continuation"
	if err = interruptRunningAttemptsTx(ctx, tx, runID, reason); err != nil {
		return err
	}
	if err = interruptActiveStagesTx(ctx, tx, runID, reason); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET state=?,ended_at=? WHERE id=? AND state=?`, model.Cancelled, at.Format(time.RFC3339Nano), runID, model.Paused); err != nil {
		return err
	}
	if err = finalizeTerminalGatesTx(ctx, tx, runID, model.Cancelled); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, runID, "", "run.cancelled", "warning", map[string]any{"reason": reason, "recovered": true}); err != nil {
		return err
	}
	for _, table := range []string{"workspace_locks", "run_owners", "run_controls"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE run_id=?`, runID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func interruptActiveStagesTx(ctx context.Context, tx *sql.Tx, runID, reason string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,stage_key,attempt FROM stages WHERE run_id=? AND state=? ORDER BY position,attempt`, runID, model.StageRun)
	if err != nil {
		return err
	}
	type activeStage struct {
		id, key string
		attempt int
	}
	var active []activeStage
	for rows.Next() {
		var item activeStage
		if err = rows.Scan(&item.id, &item.key, &item.attempt); err != nil {
			rows.Close()
			return err
		}
		active = append(active, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, item := range active {
		result, updateErr := tx.ExecContext(ctx, `UPDATE stages SET state=? WHERE id=? AND state=?`, model.StageFail, item.id, model.StageRun)
		if updateErr != nil {
			return updateErr
		}
		changed, updateErr := result.RowsAffected()
		if updateErr != nil {
			return updateErr
		}
		if changed != 1 {
			return fmt.Errorf("INVALID_STAGE_TRANSITION: abandoned stage changed concurrently")
		}
		if err = appendEvent(ctx, tx, runID, item.key, "stage.failed", "error", map[string]any{"attempt": item.attempt, "reason": reason, "interrupted": true}); err != nil {
			return err
		}
	}
	return nil
}
