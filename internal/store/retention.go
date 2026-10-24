package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/spectide/spectide/internal/model"
)

const (
	maxDetailedRuns = 1000
	detailRetention = 90 * 24 * time.Hour
)

func reserveRunDetailSlot(ctx context.Context, tx *sql.Tx, at time.Time) error {
	if err := expireOldDetails(ctx, tx, at); err != nil {
		return err
	}
	for {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE details_pruned_at IS NULL`).Scan(&count); err != nil {
			return err
		}
		if count < maxDetailedRuns {
			return nil
		}
		var id string
		err := tx.QueryRowContext(ctx, `SELECT id FROM runs WHERE details_pruned_at IS NULL AND state IN (?,?,?,?) ORDER BY COALESCE(ended_at,created_at),created_at,id LIMIT 1`, model.Completed, model.Blocked, model.Failed, model.Cancelled).Scan(&id)
		if err == sql.ErrNoRows {
			return fmt.Errorf("RETENTION_CAP_ACTIVE: all %d detailed-history slots belong to nonterminal runs; finish or cancel a run before creating another", maxDetailedRuns)
		}
		if err != nil {
			return err
		}
		if err = pruneRunDetails(ctx, tx, id, at); err != nil {
			return err
		}
	}
}

func expireOldDetails(ctx context.Context, tx *sql.Tx, at time.Time) error {
	cutoff := at.Add(-detailRetention).Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE events SET payload_json='{}' WHERE timestamp < ? AND run_id IN (SELECT id FROM runs WHERE state IN (?,?,?,?))`, cutoff, model.Completed, model.Blocked, model.Failed, model.Cancelled); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE checkpoints SET state_json='{}' WHERE created_at < ? AND run_id IN (SELECT id FROM runs WHERE state IN (?,?,?,?))`, cutoff, model.Completed, model.Blocked, model.Failed, model.Cancelled); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE execution_attempts SET error='',evidence_json='{}' WHERE finished_at < ? AND result != 'RUNNING' AND run_id IN (SELECT id FROM runs WHERE state IN (?,?,?,?)) AND (error != '' OR evidence_json != '{}')`, cutoff, model.Completed, model.Blocked, model.Failed, model.Cancelled); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE quality_results SET command_json='[]',summary='' WHERE started_at < ? AND run_id IN (SELECT id FROM runs WHERE state IN (?,?,?,?)) AND (command_json != '[]' OR summary != '')`, cutoff, model.Completed, model.Blocked, model.Failed, model.Cancelled)
	return err
}

func pruneRunDetails(ctx context.Context, tx *sql.Tx, runID string, at time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE events SET payload_json='{}' WHERE run_id=?`, runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE checkpoints SET state_json='{}' WHERE run_id=?`, runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE execution_attempts SET error='',evidence_json='{}' WHERE run_id=? AND result != 'RUNNING' AND (error != '' OR evidence_json != '{}')`, runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE quality_results SET command_json='[]',summary='' WHERE run_id=? AND (command_json != '[]' OR summary != '')`, runID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE runs SET details_pruned_at=? WHERE id=? AND state IN (?,?,?,?) AND details_pruned_at IS NULL`, at.Format(time.RFC3339Nano), runID, model.Completed, model.Blocked, model.Failed, model.Cancelled)
	return err
}

// PruneRetention expires old activity and execution detail and enforces the
// detailed-history cap while preserving summaries, outcomes, routes, gates,
// approvals, and sanitized report metadata.
func (s *Store) PruneRetention(ctx context.Context, at time.Time) error {
	if at.IsZero() {
		at = now()
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = expireOldDetails(ctx, tx, at); err != nil {
		return err
	}
	for {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE details_pruned_at IS NULL`).Scan(&count); err != nil {
			return err
		}
		if count <= maxDetailedRuns {
			break
		}
		var id string
		err = tx.QueryRowContext(ctx, `SELECT id FROM runs WHERE details_pruned_at IS NULL AND state IN (?,?,?,?) ORDER BY COALESCE(ended_at,created_at),created_at,id LIMIT 1`, model.Completed, model.Blocked, model.Failed, model.Cancelled).Scan(&id)
		if err == sql.ErrNoRows {
			return fmt.Errorf("RETENTION_CAP_ACTIVE: all detailed-history slots belong to nonterminal runs")
		}
		if err != nil {
			return err
		}
		if err = pruneRunDetails(ctx, tx, id, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RunRetentionMaintenance reconciles abandoned owners and keeps age-based
// expiration running even when no new workflow is created. Errors are reported
// to onError and do not stop future maintenance ticks.
func (s *Store) RunRetentionMaintenance(ctx context.Context, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	recoveryTicker := time.NewTicker(time.Second)
	defer recoveryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-recoveryTicker.C:
			if err := s.RecoverAbandonedRuns(ctx, at.UTC()); err != nil && onError != nil {
				onError(err)
			}
		case at := <-ticker.C:
			if err := s.PruneRetention(ctx, at.UTC()); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}
