package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spectide/spectide/internal/model"
)

func (s *Store) SaveArtifact(ctx context.Context, a model.Artifact) (model.Artifact, error) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.RunID == "" || a.Stage == "" || a.Kind == "" || a.Path == "" {
		return a, fmt.Errorf("ARTIFACT_FIELDS_REQUIRED")
	}
	clean := filepath.Clean(a.Path)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return a, fmt.Errorf("ARTIFACT_PATH_MUST_BE_WORKSPACE_RELATIVE")
	}
	if len(a.SHA256) != sha256.Size*2 {
		return a, fmt.Errorf("ARTIFACT_SHA256_REQUIRED")
	}
	if _, err := hex.DecodeString(a.SHA256); err != nil {
		return a, fmt.Errorf("ARTIFACT_SHA256_INVALID")
	}
	if a.ObservedAt.IsZero() {
		a.ObservedAt = now()
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return a, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO artifacts(id,run_id,stage,path,kind,sha256,observed_at) VALUES(?,?,?,?,?,?,?)`, a.ID, a.RunID, a.Stage, filepath.ToSlash(clean), a.Kind, strings.ToLower(a.SHA256), a.ObservedAt.Format(time.RFC3339Nano))
	if err != nil {
		return a, err
	}
	return a, tx.Commit()
}

func (s *Store) Artifacts(ctx context.Context, runID string) ([]model.Artifact, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,run_id,stage,path,kind,sha256,observed_at FROM artifacts WHERE run_id=? ORDER BY observed_at,id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Artifact
	for rows.Next() {
		var a model.Artifact
		var timestamp string
		if err = rows.Scan(&a.ID, &a.RunID, &a.Stage, &a.Path, &a.Kind, &a.SHA256, &timestamp); err != nil {
			return nil, err
		}
		a.ObservedAt, _ = time.Parse(time.RFC3339Nano, timestamp)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SaveCheckpoint(ctx context.Context, c model.Checkpoint) (model.Checkpoint, error) {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.RunID == "" || c.Stage == "" || c.WorkspaceFingerprint == "" || c.StateJSON == "" {
		return c, fmt.Errorf("CHECKPOINT_FIELDS_REQUIRED")
	}
	var state any
	if err := json.Unmarshal([]byte(c.StateJSON), &state); err != nil {
		return c, fmt.Errorf("CHECKPOINT_STATE_INVALID: %w", err)
	}
	clean, err := json.Marshal(redactValue(state))
	if err != nil {
		return c, err
	}
	c.StateJSON = string(clean)
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now()
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO checkpoints(id,run_id,stage,state_json,workspace_fingerprint,created_at) VALUES(?,?,?,?,?,?)`, c.ID, c.RunID, c.Stage, c.StateJSON, c.WorkspaceFingerprint, c.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return c, err
	}
	return c, tx.Commit()
}

func (s *Store) Checkpoints(ctx context.Context, runID string) ([]model.Checkpoint, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,run_id,stage,state_json,workspace_fingerprint,created_at FROM checkpoints WHERE run_id=? ORDER BY created_at,id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Checkpoint
	for rows.Next() {
		var c model.Checkpoint
		var timestamp string
		if err = rows.Scan(&c.ID, &c.RunID, &c.Stage, &c.StateJSON, &c.WorkspaceFingerprint, &timestamp); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, timestamp)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateApproval(ctx context.Context, a model.Approval) (model.Approval, error) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.RunID == "" || a.Action == "" {
		return a, fmt.Errorf("APPROVAL_FIELDS_REQUIRED")
	}
	if a.Status == "" {
		a.Status = "PENDING"
	}
	if a.Status != "PENDING" {
		return a, fmt.Errorf("INVALID_INITIAL_APPROVAL_STATUS")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now()
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return a, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO approvals(id,run_id,action,status,decision,created_at,resolved_at) VALUES(?,?,?,?,?,?,NULL)`, a.ID, a.RunID, a.Action, a.Status, "", a.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return a, err
	}
	return a, tx.Commit()
}

func (s *Store) ResolveApproval(ctx context.Context, id, decision string) error {
	if decision != "APPROVED" && decision != "DENIED" {
		return fmt.Errorf("INVALID_APPROVAL_DECISION")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE approvals SET status=?,decision=?,resolved_at=? WHERE id=? AND status='PENDING'`, decision, decision, now().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("APPROVAL_NOT_PENDING")
	}
	return tx.Commit()
}

func (s *Store) Approvals(ctx context.Context, runID string) ([]model.Approval, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,run_id,action,status,decision,created_at,COALESCE(resolved_at,'') FROM approvals WHERE run_id=? ORDER BY created_at,id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Approval
	for rows.Next() {
		var a model.Approval
		var created, resolved string
		if err = rows.Scan(&a.ID, &a.RunID, &a.Action, &a.Status, &a.Decision, &created, &resolved); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if resolved != "" {
			t, parseErr := time.Parse(time.RFC3339Nano, resolved)
			if parseErr == nil {
				a.ResolvedAt = &t
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
