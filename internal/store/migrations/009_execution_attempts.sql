CREATE TABLE IF NOT EXISTS execution_attempts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  stage TEXT NOT NULL,
  phase TEXT NOT NULL,
  iteration INTEGER NOT NULL,
  provider TEXT NOT NULL,
  profile TEXT NOT NULL,
  model TEXT NOT NULL,
  effort TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  start_event_seq INTEGER NOT NULL,
  completion_event_seq INTEGER NOT NULL DEFAULT 0,
  process_id INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  exit_code INTEGER NOT NULL DEFAULT 0,
  result TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  stdout_sha256 TEXT NOT NULL DEFAULT '',
  stderr_sha256 TEXT NOT NULL DEFAULT '',
  evidence_json TEXT NOT NULL DEFAULT '{}',
  artifact_ids_json TEXT NOT NULL DEFAULT '[]',
  UNIQUE(run_id,stage,phase,iteration)
);
CREATE INDEX IF NOT EXISTS execution_attempts_history ON execution_attempts(run_id,started_at,id);
CREATE TRIGGER IF NOT EXISTS execution_attempts_start_running
BEFORE INSERT ON execution_attempts
WHEN NEW.result != 'RUNNING'
BEGIN
  SELECT RAISE(ABORT, 'EXECUTION_ATTEMPT_MUST_START_RUNNING');
END;
CREATE TRIGGER IF NOT EXISTS execution_attempts_immutable_identity
BEFORE UPDATE ON execution_attempts
WHEN OLD.result != 'RUNNING' OR NEW.result NOT IN ('SUCCEEDED','FAILED','CANCELLED')
  OR NEW.id != OLD.id OR NEW.run_id != OLD.run_id OR NEW.stage != OLD.stage OR NEW.phase != OLD.phase
  OR NEW.iteration != OLD.iteration OR NEW.provider != OLD.provider OR NEW.profile != OLD.profile
  OR NEW.model != OLD.model OR NEW.effort != OLD.effort OR NEW.started_at != OLD.started_at
  OR NEW.start_event_seq != OLD.start_event_seq
BEGIN
  SELECT RAISE(ABORT, 'IMMUTABLE_EXECUTION_ATTEMPT');
END;
CREATE TRIGGER IF NOT EXISTS execution_attempts_no_delete
BEFORE DELETE ON execution_attempts
BEGIN
  SELECT RAISE(ABORT, 'APPEND_ONLY_EXECUTION_ATTEMPT');
END;

CREATE TABLE IF NOT EXISTS quality_executions (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  phase TEXT NOT NULL,
  iteration INTEGER NOT NULL,
  process_id INTEGER NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NOT NULL,
  completion_event_seq INTEGER NOT NULL,
  verdict TEXT NOT NULL,
  UNIQUE(run_id,phase,iteration)
);
CREATE TABLE IF NOT EXISTS quality_results (
  execution_id TEXT NOT NULL REFERENCES quality_executions(id),
  run_id TEXT NOT NULL REFERENCES runs(id),
  check_key TEXT NOT NULL,
  verdict TEXT NOT NULL,
  command_json TEXT NOT NULL,
  process_id INTEGER NOT NULL DEFAULT 0,
  exit_code INTEGER NOT NULL,
  duration_ns INTEGER NOT NULL,
  summary TEXT NOT NULL,
  started_at TEXT NOT NULL,
  PRIMARY KEY(execution_id,check_key)
);
CREATE INDEX IF NOT EXISTS quality_execution_history ON quality_executions(run_id,started_at,id);
CREATE TRIGGER IF NOT EXISTS quality_executions_no_update
BEFORE UPDATE ON quality_executions
BEGIN
  SELECT RAISE(ABORT, 'APPEND_ONLY_QUALITY_EXECUTION');
END;
CREATE TRIGGER IF NOT EXISTS quality_results_no_update
BEFORE UPDATE ON quality_results
BEGIN
  SELECT RAISE(ABORT, 'APPEND_ONLY_QUALITY_RESULT');
END;
CREATE TRIGGER IF NOT EXISTS quality_executions_no_delete
BEFORE DELETE ON quality_executions
BEGIN
  SELECT RAISE(ABORT, 'APPEND_ONLY_QUALITY_EXECUTION');
END;
CREATE TRIGGER IF NOT EXISTS quality_results_no_delete
BEFORE DELETE ON quality_results
BEGIN
  SELECT RAISE(ABORT, 'APPEND_ONLY_QUALITY_RESULT');
END;
