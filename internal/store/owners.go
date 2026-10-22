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

// RecoverAbandonedRuns blocks running or paused work only after its persisted
// owner lease expires. Restart resume remains denied until checkpoint, artifact,
// Git HEAD, and managed-path fingerprints can all be verified.
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
