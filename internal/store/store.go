package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/security"
	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Keep connection-scoped SQLite pragmas and write ordering consistent.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{DB: db}
	return s, s.migrate()
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return err
	}
	if version < 1 {
		if _, err = tx.Exec(schemaV1); err != nil {
			return fmt.Errorf("apply schema migration 1: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(1,?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

//go:embed migrations/001_initial.sql
var schemaV1 string

func now() time.Time { return time.Now().UTC().Round(0) }
func parse(v string) *time.Time {
	if v == "" {
		return nil
	}
	t, e := time.Parse(time.RFC3339Nano, v)
	if e != nil {
		return nil
	}
	return &t
}

func (s *Store) CreateRun(ctx context.Context, project, prompt string) (model.Run, error) {
	prompt = security.Redact(prompt)
	t := now()
	r := model.Run{ID: uuid.NewString(), Project: project, Prompt: prompt, State: model.Created, CreatedAt: t}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO runs VALUES(?,?,?,?,?,?,?)`, r.ID, r.Project, r.Prompt, r.State, t.Format(time.RFC3339Nano), nil, nil)
	if err != nil {
		return r, err
	}
	if err = appendEvent(ctx, tx, r.ID, "", "run.created", "info", map[string]any{"project": project}); err != nil {
		return r, err
	}
	return r, tx.Commit()
}

func appendEvent(ctx context.Context, tx *sql.Tx, runID, stage, typ, severity string, payload map[string]any) error {
	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM events WHERE run_id=?`, runID).Scan(&seq); err != nil {
		return err
	}
	b, err := json.Marshal(redactValue(payload))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO events VALUES(?,?,?,?,?,?,?)`, runID, seq, now().Format(time.RFC3339Nano), stage, typ, severity, b)
	return err
}

func redactValue(v any) any {
	switch x := v.(type) {
	case string:
		return security.Redact(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			if sensitivePayloadKey(k) {
				out[k] = "[REDACTED]"
				continue
			}
			out[k] = redactValue(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = redactValue(v)
		}
		return out
	default:
		return v
	}
}

func sensitivePayloadKey(key string) bool {
	k := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", ""), "_", ""))
	return k == "token" || k == "apikey" || k == "authorization" || k == "secret" || k == "password" || k == "credential"
}

func (s *Store) Event(ctx context.Context, runID, stage, typ, severity string, payload map[string]any) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = appendEvent(ctx, tx, runID, stage, typ, severity, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetRunState(ctx context.Context, id string, state model.RunState) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current model.RunState
	if err = tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE id=?`, id).Scan(&current); err != nil {
		return err
	}
	if !validTransition(current, state) {
		return fmt.Errorf("INVALID_RUN_TRANSITION: %s -> %s", current, state)
	}
	t := now()
	var started, ended any
	if state == model.Running {
		started = t.Format(time.RFC3339Nano)
	}
	if state == model.Completed || state == model.Blocked || state == model.Failed || state == model.Cancelled {
		ended = t.Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx, `UPDATE runs SET state=?, started_at=COALESCE(started_at,?), ended_at=COALESCE(?,ended_at) WHERE id=?`, state, started, ended, id)
	if err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, id, "", "run."+string(state), "info", map[string]any{"state": state}); err != nil {
		return err
	}
	return tx.Commit()
}

func validTransition(from, to model.RunState) bool {
	switch from {
	case model.Created:
		return to == model.Running || to == model.Blocked || to == model.Cancelled
	case model.Running:
		return to == model.Paused || to == model.Completed || to == model.Blocked || to == model.Failed || to == model.Cancelled
	case model.Paused:
		return to == model.Running || to == model.Cancelled || to == model.Blocked || to == model.Failed
	default:
		return false
	}
}

// RecoverInterrupted closes runs whose owner disappeared before the next process
// started. They stay blocked until a future implementation can verify a safe resume.
func (s *Store) RecoverInterrupted(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM runs WHERE state=?`, model.Running)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE runs SET state=?,ended_at=? WHERE id=? AND state=?`, model.Blocked, now().Format(time.RFC3339Nano), id, model.Running)
		if e == nil {
			e = appendEvent(ctx, tx, id, "", "run.interrupted", "warning", map[string]any{"reason": "owner process exited; safe resume is not verified"})
		}
		if e != nil {
			tx.Rollback()
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM workspace_locks WHERE run_id NOT IN (SELECT id FROM runs WHERE state=?)`, model.Running)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) RegisterProject(ctx context.Context, path, name string, scopes []string) error {
	if path == "" || name == "" {
		return fmt.Errorf("PROJECT_PATH_AND_NAME_REQUIRED")
	}
	b, err := json.Marshal(scopes)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO projects(id,canonical_path,display_name,allowed_scopes_json,created_at) VALUES(?,?,?,?,?) ON CONFLICT(canonical_path) DO UPDATE SET display_name=excluded.display_name,allowed_scopes_json=excluded.allowed_scopes_json`, uuid.NewString(), path, name, string(b), now().Format(time.RFC3339Nano))
	return err
}

func (s *Store) AcquireWorkspaceLock(ctx context.Context, path, runID string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO workspace_locks(canonical_path,run_id,acquired_at) VALUES(?,?,?)`, path, runID, now().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("WORKSPACE_LOCKED: %s", path)
	}
	return nil
}

func (s *Store) ReleaseWorkspaceLock(ctx context.Context, path, runID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM workspace_locks WHERE canonical_path=? AND run_id=?`, path, runID)
	return err
}

func (s *Store) SaveRoutingDecision(ctx context.Context, runID, stage, provider, profile, modelName, effort, policy, explanation string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO routing_decisions(run_id,stage,provider,profile,model,effort,policy,explanation,created_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(run_id,stage) DO UPDATE SET provider=excluded.provider,profile=excluded.profile,model=excluded.model,effort=excluded.effort,policy=excluded.policy,explanation=excluded.explanation,created_at=excluded.created_at`, runID, stage, provider, profile, modelName, effort, policy, security.Redact(explanation), now().Format(time.RFC3339Nano))
	return err
}

func (s *Store) RoutingDecisions(ctx context.Context, runID string) ([]model.RoutingDecision, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT stage,provider,profile,model,effort,policy,explanation FROM routing_decisions WHERE run_id=? ORDER BY created_at,stage`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.RoutingDecision
	for rows.Next() {
		var d model.RoutingDecision
		if err = rows.Scan(&d.Stage, &d.Provider, &d.Profile, &d.Model, &d.Effort, &d.Policy, &d.Explanation); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) AddStage(ctx context.Context, st model.Stage) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO stages VALUES(?,?,?,?,?,?,?,?,?,?)`, st.ID, st.RunID, st.Key, st.Position, st.State, st.Provider, st.Profile, st.Model, st.Effort, st.Attempt)
	return err
}
func (s *Store) SetStage(ctx context.Context, id string, state model.StageState) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE stages SET state=? WHERE id=?`, state, id)
	return err
}
func scanRun(row interface{ Scan(...any) error }) (model.Run, error) {
	var r model.Run
	var c, st, en string
	err := row.Scan(&r.ID, &r.Project, &r.Prompt, &r.State, &c, &st, &en)
	if err != nil {
		return r, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
	r.StartedAt = parse(st)
	r.EndedAt = parse(en)
	return r, nil
}
func (s *Store) GetRun(ctx context.Context, id string) (model.Run, error) {
	return scanRun(s.DB.QueryRowContext(ctx, `SELECT id,project,prompt,state,created_at,COALESCE(started_at,''),COALESCE(ended_at,'') FROM runs WHERE id=?`, id))
}
func (s *Store) Runs(ctx context.Context) ([]model.Run, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,project,prompt,state,created_at,COALESCE(started_at,''),COALESCE(ended_at,'') FROM runs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Run
	for rows.Next() {
		r, e := scanRun(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) Stages(ctx context.Context, runID string) ([]model.Stage, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,run_id,stage_key,position,state,provider,profile,model,effort,attempt FROM stages WHERE run_id=? ORDER BY position,attempt`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var o []model.Stage
	for rows.Next() {
		var x model.Stage
		if e := rows.Scan(&x.ID, &x.RunID, &x.Key, &x.Position, &x.State, &x.Provider, &x.Profile, &x.Model, &x.Effort, &x.Attempt); e != nil {
			return nil, e
		}
		o = append(o, x)
	}
	return o, rows.Err()
}
func (s *Store) Events(ctx context.Context, runID string, after int64) ([]model.Event, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT seq,timestamp,stage,type,severity,payload_json FROM events WHERE run_id=? AND seq>? ORDER BY seq`, runID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var o []model.Event
	for rows.Next() {
		var e model.Event
		var ts, payload string
		if er := rows.Scan(&e.Seq, &ts, &e.Stage, &e.Type, &e.Severity, &payload); er != nil {
			return nil, er
		}
		e.RunID = runID
		e.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		if er := json.Unmarshal([]byte(payload), &e.Payload); er != nil {
			return nil, er
		}
		o = append(o, e)
	}
	return o, rows.Err()
}
func (s *Store) Gate(ctx context.Context, runID, key, verdict, evidence string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO quality_gates VALUES(?,?,?,?) ON CONFLICT(run_id,gate_key) DO UPDATE SET verdict=excluded.verdict,evidence=excluded.evidence`, runID, key, verdict, evidence)
	return err
}
func (s *Store) Gates(ctx context.Context, runID string) ([]model.Gate, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT gate_key,verdict,evidence FROM quality_gates WHERE run_id=? ORDER BY gate_key`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var o []model.Gate
	for rows.Next() {
		var g model.Gate
		if e := rows.Scan(&g.Key, &g.Verdict, &g.Evidence); e != nil {
			return nil, e
		}
		o = append(o, g)
	}
	return o, rows.Err()
}
func (s *Store) SaveProfile(ctx context.Context, p model.Profile) (model.Profile, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Name == "" || p.Provider == "" || p.Executable == "" {
		return p, fmt.Errorf("profile name, provider, and executable are required")
	}
	scopes, e := json.Marshal(p.AllowedScopes)
	if e != nil {
		return p, e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO profiles(id,name,provider,executable,config_directory,allowed_scopes_json) VALUES(?,?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET provider=excluded.provider,executable=excluded.executable,config_directory=excluded.config_directory,allowed_scopes_json=excluded.allowed_scopes_json`, p.ID, p.Name, p.Provider, p.Executable, p.ConfigDirectory, string(scopes))
	return p, e
}
func (s *Store) Profiles(ctx context.Context) ([]model.Profile, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,name,provider,executable,config_directory,allowed_scopes_json FROM profiles ORDER BY name`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Profile
	for rows.Next() {
		var p model.Profile
		var scopes string
		if e = rows.Scan(&p.ID, &p.Name, &p.Provider, &p.Executable, &p.ConfigDirectory, &scopes); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(scopes), &p.AllowedScopes); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) Report(ctx context.Context, id string) (model.Report, error) {
	r, e := s.GetRun(ctx, id)
	if e != nil {
		return model.Report{}, fmt.Errorf("run: %w", e)
	}
	st, e := s.Stages(ctx, id)
	if e != nil {
		return model.Report{}, e
	}
	g, e := s.Gates(ctx, id)
	if e != nil {
		return model.Report{}, e
	}
	d, e := s.RoutingDecisions(ctx, id)
	return model.Report{Run: r, Stages: st, Decisions: d, Gates: g}, e
}
