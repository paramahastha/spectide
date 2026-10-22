package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spectide/spectide/internal/events"
	"github.com/spectide/spectide/internal/model"
	"github.com/spectide/spectide/internal/security"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB   *sql.DB
	path string
}

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
	absPath, absErr := filepath.Abs(path)
	if absErr != nil {
		db.Close()
		return nil, absErr
	}
	s := &Store{DB: db, path: absPath}
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
		version = 1
	}
	if version < 2 {
		if _, err = tx.Exec(schemaV2); err != nil {
			return fmt.Errorf("apply schema migration 2: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(2,?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		version = 2
	}
	if version < 3 {
		if _, err = tx.Exec(schemaV3); err != nil {
			return fmt.Errorf("apply schema migration 3: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(3,?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if version < 4 {
		if _, err = tx.Exec(schemaV4); err != nil {
			return fmt.Errorf("apply schema migration 4: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(4,?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		version = 4
	}
	if version < 5 {
		if _, err = tx.Exec(schemaV5); err != nil {
			return fmt.Errorf("apply schema migration 5: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(5,?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		version = 5
	}
	if version < 6 {
		if _, err = tx.Exec(schemaV6); err != nil {
			return fmt.Errorf("apply schema migration 6: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(6,?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		version = 6
	}
	if version < 7 {
		if _, err = tx.Exec(schemaV7); err != nil {
			return fmt.Errorf("apply schema migration 7: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(7,?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		version = 7
	}
	if version < 8 {
		if _, err = tx.Exec(schemaV8); err != nil {
			return fmt.Errorf("apply schema migration 8: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(8,?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

//go:embed migrations/001_initial.sql
var schemaV1 string

//go:embed migrations/002_idempotency.sql
var schemaV2 string

//go:embed migrations/003_run_controls.sql
var schemaV3 string

//go:embed migrations/004_run_configuration.sql
var schemaV4 string

//go:embed migrations/005_retention.sql
var schemaV5 string

//go:embed migrations/006_run_owners.sql
var schemaV6 string

//go:embed migrations/007_profile_capabilities.sql
var schemaV7 string

//go:embed migrations/008_stage_running_state.sql
var schemaV8 string

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
	r, _, err := s.CreateRunIdempotent(ctx, "", project, prompt)
	return r, err
}

func (s *Store) CreateRunIdempotent(ctx context.Context, key, project, prompt string) (model.Run, bool, error) {
	return s.CreateRunWithConfiguration(ctx, key, project, prompt, nil)
}

func (s *Store) CreateRunWithConfiguration(ctx context.Context, key, project, prompt string, configJSON []byte) (model.Run, bool, error) {
	if len(configJSON) > 0 && security.Redact(string(configJSON)) != string(configJSON) {
		return model.Run{}, false, fmt.Errorf("RUN_CONFIGURATION_SECRET_REFERENCE_REJECTED")
	}
	requestHash := ""
	if key != "" {
		sum := sha256.Sum256([]byte(project + "\x00" + prompt + "\x00" + string(configJSON)))
		requestHash = hex.EncodeToString(sum[:])
	}
	prompt = security.Redact(prompt)
	configHash := ""
	if len(configJSON) > 0 {
		if !json.Valid(configJSON) {
			return model.Run{}, false, fmt.Errorf("RUN_CONFIGURATION_INVALID_JSON")
		}
		sum := sha256.Sum256(configJSON)
		configHash = hex.EncodeToString(sum[:])
	}
	t := now()
	r := model.Run{ID: uuid.NewString(), Project: project, Prompt: prompt, State: model.Created, CreatedAt: t}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return r, false, err
	}
	defer tx.Rollback()
	if key != "" {
		var existing model.Run
		var existingHash string
		var created, started, ended string
		err = tx.QueryRowContext(ctx, `SELECT i.request_hash,r.id,r.project,r.prompt,r.state,r.created_at,COALESCE(r.started_at,''),COALESCE(r.ended_at,'') FROM idempotency_keys i JOIN runs r ON r.id=i.run_id WHERE i.key=?`, key).Scan(&existingHash, &existing.ID, &existing.Project, &existing.Prompt, &existing.State, &created, &started, &ended)
		if err == nil {
			if existingHash != requestHash {
				return model.Run{}, false, fmt.Errorf("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
			}
			existing.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
			existing.StartedAt, existing.EndedAt = parse(started), parse(ended)
			return existing, true, nil
		} else if err != sql.ErrNoRows {
			return model.Run{}, false, err
		}
	}
	if err = reserveRunDetailSlot(ctx, tx, t); err != nil {
		return model.Run{}, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runs(id,project,prompt,state,created_at,started_at,ended_at) VALUES(?,?,?,?,?,?,?)`, r.ID, r.Project, r.Prompt, r.State, t.Format(time.RFC3339Nano), nil, nil)
	if err != nil {
		return r, false, err
	}
	if len(configJSON) > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO run_configurations(run_id,config_json,config_sha256,created_at) VALUES(?,?,?,?)`, r.ID, string(configJSON), configHash, t.Format(time.RFC3339Nano)); err != nil {
			return r, false, err
		}
	}
	if key != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO idempotency_keys(key,request_hash,run_id,created_at) VALUES(?,?,?,?)`, key, requestHash, r.ID, t.Format(time.RFC3339Nano)); err != nil {
			return r, false, err
		}
	}
	if err = appendEvent(ctx, tx, r.ID, "", "run.created", "info", map[string]any{"project": project}); err != nil {
		return r, false, err
	}
	return r, false, tx.Commit()
}

func (s *Store) RunConfiguration(ctx context.Context, runID string) (model.RunConfiguration, error) {
	var c model.RunConfiguration
	var created string
	err := s.DB.QueryRowContext(ctx, `SELECT run_id,config_json,config_sha256,created_at FROM run_configurations WHERE run_id=?`, runID).Scan(&c.RunID, &c.ConfigJSON, &c.SHA256, &created)
	if err != nil {
		return c, err
	}
	actual := sha256.Sum256([]byte(c.ConfigJSON))
	if hex.EncodeToString(actual[:]) != c.SHA256 {
		return model.RunConfiguration{}, fmt.Errorf("RUN_CONFIGURATION_HASH_MISMATCH")
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return c, nil
}

func appendEvent(ctx context.Context, tx *sql.Tx, runID, stage, typ, severity string, payload map[string]any) error {
	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM events WHERE run_id=?`, runID).Scan(&seq); err != nil {
		return err
	}
	b, err := json.Marshal(events.SanitizePayload(payload))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO events VALUES(?,?,?,?,?,?,?)`, runID, seq, now().Format(time.RFC3339Nano), stage, typ, severity, b)
	return err
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

// RecoverInterrupted is retained for callers that explicitly request recovery;
// it honors owner leases and never interrupts a heartbeat-fresh worker.
func (s *Store) RecoverInterrupted(ctx context.Context) error {
	return s.RecoverAbandonedRuns(ctx, now())
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

func (s *Store) Projects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,canonical_path,display_name,allowed_scopes_json FROM projects ORDER BY display_name,canonical_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Project
	for rows.Next() {
		var project model.Project
		var scopes string
		if err = rows.Scan(&project.ID, &project.CanonicalPath, &project.DisplayName, &scopes); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(scopes), &project.AllowedScopes); err != nil {
			return nil, err
		}
		result = append(result, project)
	}
	return result, rows.Err()
}

func (s *Store) Project(ctx context.Context, id string) (model.Project, error) {
	var project model.Project
	var scopes string
	err := s.DB.QueryRowContext(ctx, `SELECT id,canonical_path,display_name,allowed_scopes_json FROM projects WHERE id=?`, id).Scan(&project.ID, &project.CanonicalPath, &project.DisplayName, &scopes)
	if err != nil {
		return project, err
	}
	err = json.Unmarshal([]byte(scopes), &project.AllowedScopes)
	return project, err
}

func (s *Store) AcquireWorkspaceLock(ctx context.Context, path, runID string) error {
	return s.AcquireWorkspaceLockOwned(ctx, path, runID, "")
}

func (s *Store) AcquireWorkspaceLockOwned(ctx context.Context, path, runID, ownerID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO workspace_locks(canonical_path,run_id,acquired_at) VALUES(?,?,?)`, path, runID, now().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("WORKSPACE_LOCKED: %s", path)
	}
	if ownerID != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO run_owners(run_id,owner_id,heartbeat_at) VALUES(?,?,?)`, runID, ownerID, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ReleaseWorkspaceLock(ctx context.Context, path, runID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM workspace_locks WHERE canonical_path=? AND run_id=?`, path, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM run_owners WHERE run_id=?`, runID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveRoutingDecision(ctx context.Context, runID, stage, provider, profile, modelName, effort, policy, explanation string, simulatedOverride ...bool) error {
	for _, identity := range []string{provider, profile, modelName, effort, policy} {
		if security.Redact(identity) != identity {
			return fmt.Errorf("ROUTING_SECRET_REFERENCE_REJECTED")
		}
	}
	explanation = security.Redact(explanation)
	simulated := provider == "fake"
	if len(simulatedOverride) > 0 {
		simulated = simulatedOverride[0]
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO routing_decisions(run_id,stage,provider,profile,model,effort,policy,explanation,created_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(run_id,stage) DO UPDATE SET provider=excluded.provider,profile=excluded.profile,model=excluded.model,effort=excluded.effort,policy=excluded.policy,explanation=excluded.explanation,created_at=excluded.created_at`, runID, stage, provider, profile, modelName, effort, policy, explanation, now().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, runID, stage, "routing.resolved", "info", map[string]any{"provider": provider, "profile": profile, "model": modelName, "effort": effort, "policy": policy, "explanation": explanation, "simulated": simulated}); err != nil {
		return err
	}
	return tx.Commit()
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
	if st.Attempt < 1 || st.ID == "" || st.RunID == "" || st.Key == "" {
		return fmt.Errorf("INVALID_STAGE_ATTEMPT")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previousAttempt int
	var previousState model.StageState
	var position int
	var provider, profile, modelName, effort string
	err = tx.QueryRowContext(ctx, `SELECT attempt,state,position,provider,profile,model,effort FROM stages WHERE run_id=? AND stage_key=? ORDER BY attempt DESC LIMIT 1`, st.RunID, st.Key).Scan(&previousAttempt, &previousState, &position, &provider, &profile, &modelName, &effort)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if st.Attempt != previousAttempt+1 {
			return fmt.Errorf("STAGE_ATTEMPT_NOT_APPEND_ONLY: expected %d", previousAttempt+1)
		}
		if previousState == model.Pending || previousState == model.StageRun {
			return fmt.Errorf("STAGE_ATTEMPT_STILL_ACTIVE")
		}
		if st.Position != position || st.Provider != provider || st.Profile != profile || st.Model != modelName || st.Effort != effort {
			return fmt.Errorf("STAGE_ATTEMPT_CONFIGURATION_CHANGED")
		}
	} else if st.Attempt != 1 {
		return fmt.Errorf("STAGE_ATTEMPT_NOT_APPEND_ONLY: first attempt must be 1")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO stages VALUES(?,?,?,?,?,?,?,?,?,?)`, st.ID, st.RunID, st.Key, st.Position, st.State, st.Provider, st.Profile, st.Model, st.Effort, st.Attempt); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, st.RunID, st.Key, "stage.created", "info", map[string]any{"attempt": st.Attempt, "state": st.State, "provider": st.Provider, "profile": st.Profile, "model": st.Model, "effort": st.Effort}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) SetStage(ctx context.Context, id string, state model.StageState) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current model.StageState
	var runID, stageKey string
	var attempt int
	if err = tx.QueryRowContext(ctx, `SELECT state,run_id,stage_key,attempt FROM stages WHERE id=?`, id).Scan(&current, &runID, &stageKey, &attempt); err != nil {
		return err
	}
	allowed := false
	switch current {
	case model.Pending:
		allowed = state == model.StageRun || state == model.Succeeded || state == model.StageFail || state == model.Skipped
	case model.StageRun:
		allowed = state == model.Succeeded || state == model.StageFail || state == model.Skipped
	}
	if !allowed {
		return fmt.Errorf("INVALID_STAGE_TRANSITION: %s -> %s", current, state)
	}
	result, err := tx.ExecContext(ctx, `UPDATE stages SET state=? WHERE id=? AND state=?`, state, id, current)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("INVALID_STAGE_TRANSITION: stage changed concurrently")
	}
	if err = appendEvent(ctx, tx, runID, stageKey, "stage.transition", "info", map[string]any{"attempt": attempt, "from_state": current, "state": state}); err != nil {
		return err
	}
	return tx.Commit()
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

func (s *Store) EventHighWater(ctx context.Context, runID string) (int64, error) {
	var sequence int64
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM events WHERE run_id=?`, runID).Scan(&sequence)
	return sequence, err
}

func (s *Store) EventsPage(ctx context.Context, runID string, after, through int64, limit int) ([]model.Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 256
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT seq,timestamp,stage,type,severity,payload_json FROM events WHERE run_id=? AND seq>? AND seq<=? ORDER BY seq LIMIT ?`, runID, after, through, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []model.Event
	for rows.Next() {
		var event model.Event
		var timestamp, payload string
		if err = rows.Scan(&event.Seq, &timestamp, &event.Stage, &event.Type, &event.Severity, &payload); err != nil {
			return nil, err
		}
		event.RunID = runID
		event.Timestamp, _ = time.Parse(time.RFC3339Nano, timestamp)
		if err = json.Unmarshal([]byte(payload), &event.Payload); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
func (s *Store) Gate(ctx context.Context, runID, key, verdict, evidence string) error {
	evidence = security.Redact(evidence)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO quality_gates VALUES(?,?,?,?) ON CONFLICT(run_id,gate_key) DO UPDATE SET verdict=excluded.verdict,evidence=excluded.evidence`, runID, key, verdict, evidence); err != nil {
		return err
	}
	severity := "info"
	if verdict != "PASS" {
		severity = "warning"
	}
	if err = appendEvent(ctx, tx, runID, "", "quality.gate_recorded", severity, map[string]any{"gate_key": key, "verdict": verdict, "evidence": evidence}); err != nil {
		return err
	}
	return tx.Commit()
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
	for _, identity := range append([]string{p.Name, p.Executable, p.ConfigDirectory}, p.AllowedScopes...) {
		if security.Redact(identity) != identity {
			return model.Profile{}, fmt.Errorf("PROFILE_SECRET_REFERENCE_REJECTED")
		}
	}
	switch p.Provider {
	case "claude", "codex", "opencode", "fake":
	default:
		return p, fmt.Errorf("UNSUPPORTED_PROVIDER: %s", p.Provider)
	}
	if strings.ContainsAny(p.Executable, "\x00\n\r") || (strings.ContainsRune(p.Executable, filepath.Separator) && !filepath.IsAbs(p.Executable)) {
		return p, fmt.Errorf("INVALID_EXECUTABLE: use a command name or absolute executable path")
	}
	if p.ConfigDirectory != "" && !filepath.IsAbs(p.ConfigDirectory) {
		return p, fmt.Errorf("CONFIG_DIRECTORY_MUST_BE_ABSOLUTE")
	}
	if len(p.AllowedScopes) == 0 {
		return p, fmt.Errorf("PROFILE_SCOPE_REQUIRED")
	}
	scopes, e := json.Marshal(p.AllowedScopes)
	if e != nil {
		return p, e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return p, e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO profiles(id,name,provider,executable,config_directory,allowed_scopes_json) VALUES(?,?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET provider=excluded.provider,executable=excluded.executable,config_directory=excluded.config_directory,allowed_scopes_json=excluded.allowed_scopes_json`, p.ID, p.Name, p.Provider, p.Executable, p.ConfigDirectory, string(scopes))
	if e != nil {
		return p, e
	}
	// Any profile edit invalidates its prior executable and policy observation.
	if _, e = tx.ExecContext(ctx, `DELETE FROM profile_capabilities WHERE profile_name=?`, p.Name); e != nil {
		return p, e
	}
	return p, tx.Commit()
}
func (s *Store) Profiles(ctx context.Context) ([]model.Profile, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT p.id,p.name,p.provider,p.executable,p.config_directory,p.allowed_scopes_json,COALESCE(c.capability_json,''),COALESCE(c.observed_at,'') FROM profiles p LEFT JOIN profile_capabilities c ON c.profile_name=p.name ORDER BY p.name`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Profile
	for rows.Next() {
		var p model.Profile
		var scopes, capabilityJSON, observedAt string
		if e = rows.Scan(&p.ID, &p.Name, &p.Provider, &p.Executable, &p.ConfigDirectory, &scopes, &capabilityJSON, &observedAt); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(scopes), &p.AllowedScopes); e != nil {
			return nil, e
		}
		if capabilityJSON != "" {
			var capability model.ProfileCapability
			if e = json.Unmarshal([]byte(capabilityJSON), &capability); e != nil {
				return nil, fmt.Errorf("PROFILE_CAPABILITY_INVALID: %w", e)
			}
			capability.ObservedAt, _ = time.Parse(time.RFC3339Nano, observedAt)
			p.Capability = &capability
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) SaveProfileCapability(ctx context.Context, name string, capability model.ProfileCapability) (model.ProfileCapability, error) {
	if name == "" || capability.Provider == "" || capability.Authenticated == "" {
		return capability, fmt.Errorf("PROFILE_CAPABILITY_FIELDS_REQUIRED")
	}
	switch capability.Authenticated {
	case "NOT_PROBED", "AUTHENTICATED", "UNAUTHENTICATED", "UNKNOWN":
	default:
		return capability, fmt.Errorf("PROFILE_AUTH_STATE_INVALID")
	}
	if len(capability.Version) > 512 || len(capability.Warnings) > 16 {
		return capability, fmt.Errorf("PROFILE_CAPABILITY_LIMIT_EXCEEDED")
	}
	for _, warning := range capability.Warnings {
		if len(warning) > 512 {
			return capability, fmt.Errorf("PROFILE_CAPABILITY_LIMIT_EXCEEDED")
		}
	}
	var provider string
	if err := s.DB.QueryRowContext(ctx, `SELECT provider FROM profiles WHERE name=?`, name).Scan(&provider); err != nil {
		if err == sql.ErrNoRows {
			return capability, fmt.Errorf("PROFILE_NOT_FOUND: %s", name)
		}
		return capability, err
	}
	if capability.Provider != provider {
		return capability, fmt.Errorf("PROFILE_PROVIDER_MISMATCH: %s", name)
	}
	capability.Version = security.Redact(capability.Version)
	capability.EffortOverride = security.Redact(capability.EffortOverride)
	capability.Warnings = append([]string(nil), capability.Warnings...)
	for i, warning := range capability.Warnings {
		capability.Warnings[i] = security.Redact(warning)
	}
	capability.ObservedAt = now()
	encoded, err := json.Marshal(capability)
	if err != nil {
		return capability, err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO profile_capabilities(profile_name,capability_json,observed_at) VALUES(?,?,?) ON CONFLICT(profile_name) DO UPDATE SET capability_json=excluded.capability_json,observed_at=excluded.observed_at`, name, string(encoded), capability.ObservedAt.Format(time.RFC3339Nano))
	return capability, err
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
	a, e := s.Artifacts(ctx, id)
	if e != nil {
		return model.Report{}, e
	}
	cp, e := s.Checkpoints(ctx, id)
	if e != nil {
		return model.Report{}, e
	}
	ap, e := s.Approvals(ctx, id)
	if e != nil {
		return model.Report{}, e
	}
	d, e := s.RoutingDecisions(ctx, id)
	if e != nil {
		return model.Report{}, e
	}
	var configuration *model.RunConfiguration
	c, configErr := s.RunConfiguration(ctx, id)
	if configErr == nil {
		configuration = &c
	} else if configErr != sql.ErrNoRows {
		return model.Report{}, configErr
	}
	return model.Report{Run: r, Configuration: configuration, Stages: st, Decisions: d, Gates: g, Artifacts: a, Checkpoints: cp, Approvals: ap}, nil
}
