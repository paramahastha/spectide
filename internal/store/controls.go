package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/spectide/spectide/internal/model"
)

func (s *Store) RequestRunControl(ctx context.Context, runID, action string) error {
	if action != "pause" && action != "resume" && action != "cancel" {
		return fmt.Errorf("INVALID_RUN_CONTROL")
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
	if state == model.Cancelled && action == "cancel" {
		return nil
	}
	var existing string
	controlErr := tx.QueryRowContext(ctx, `SELECT action FROM run_controls WHERE run_id=?`, runID).Scan(&existing)
	if controlErr != nil && controlErr != sql.ErrNoRows {
		return controlErr
	}
	if existing == "cancel" {
		if action == "cancel" {
			return nil
		}
		return fmt.Errorf("RUN_CANCEL_PENDING")
	}
	allowed := action == "cancel" && (state == model.Created || state == model.Running || state == model.Paused) || action == "pause" && state == model.Running || action == "resume" && state == model.Paused
	if !allowed {
		return fmt.Errorf("RUN_CONTROL_NOT_ALLOWED: %s for %s", action, state)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO run_controls(run_id,action,updated_at) VALUES(?,?,?) ON CONFLICT(run_id) DO UPDATE SET action=excluded.action,updated_at=excluded.updated_at`, runID, action, now().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if action == "cancel" {
		if err = appendEvent(ctx, tx, runID, "", "run.cancel_requested", "warning", map[string]any{"state": state}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RunControl(ctx context.Context, runID string) (string, error) {
	var action string
	err := s.DB.QueryRowContext(ctx, `SELECT action FROM run_controls WHERE run_id=?`, runID).Scan(&action)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return action, err
}

func (s *Store) ClearRunControl(ctx context.Context, runID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM run_controls WHERE run_id=?`, runID)
	return err
}

func (s *Store) ClearRunControlUnlessCancelled(ctx context.Context, runID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var action string
	err = tx.QueryRowContext(ctx, `SELECT action FROM run_controls WHERE run_id=?`, runID).Scan(&action)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if action == "cancel" {
		return fmt.Errorf("RUN_CANCEL_PENDING")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM run_controls WHERE run_id=?`, runID); err != nil {
		return err
	}
	return tx.Commit()
}
