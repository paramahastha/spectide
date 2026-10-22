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
	var state model.RunState
	if err := s.DB.QueryRowContext(ctx, `SELECT state FROM runs WHERE id=?`, runID).Scan(&state); err != nil {
		return err
	}
	allowed := action == "cancel" && (state == model.Created || state == model.Running || state == model.Paused) || action == "pause" && state == model.Running || action == "resume" && state == model.Paused
	if !allowed {
		return fmt.Errorf("RUN_CONTROL_NOT_ALLOWED: %s for %s", action, state)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO run_controls(run_id,action,updated_at) VALUES(?,?,?) ON CONFLICT(run_id) DO UPDATE SET action=excluded.action,updated_at=excluded.updated_at`, runID, action, now().Format(time.RFC3339Nano))
	return err
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
