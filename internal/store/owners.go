package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/spectide/spectide/internal/model"
)

const RunOwnerLease = 3 * time.Second

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
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.state,COALESCE(o.heartbeat_at,'') FROM runs r LEFT JOIN run_owners o ON o.run_id=r.id WHERE r.state IN (?,?)`, model.Running, model.Paused)
	if err != nil {
		return err
	}
	type ownerState struct {
		id    string
		state model.RunState
	}
	var stale []ownerState
	for rows.Next() {
		var id, heartbeat string
		var state model.RunState
		if err = rows.Scan(&id, &state, &heartbeat); err != nil {
			rows.Close()
			return err
		}
		updated, parseErr := time.Parse(time.RFC3339Nano, heartbeat)
		if heartbeat == "" || parseErr != nil || updated.Before(cutoff) {
			stale = append(stale, ownerState{id: id, state: state})
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range stale {
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
		result, txErr := tx.ExecContext(ctx, `UPDATE runs SET state=?,ended_at=? WHERE id=? AND state=?`, model.Blocked, at.Format(time.RFC3339Nano), item.id, item.state)
		if txErr == nil {
			var changed int64
			changed, txErr = result.RowsAffected()
			if txErr == nil && changed == 1 {
				txErr = appendEvent(ctx, tx, item.id, "", "run.interrupted", "warning", map[string]any{"reason": "WORKSPACE_DRIFT: owner lease expired without verified checkpoint, artifact, and Git fingerprint recovery"})
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
	return nil
}
