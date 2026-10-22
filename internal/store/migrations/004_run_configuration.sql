CREATE TABLE IF NOT EXISTS run_configurations (
  run_id TEXT PRIMARY KEY REFERENCES runs(id),
  config_json TEXT NOT NULL,
  config_sha256 TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TRIGGER IF NOT EXISTS run_configurations_no_update
BEFORE UPDATE ON run_configurations
BEGIN
  SELECT RAISE(ABORT, 'IMMUTABLE_RUN_CONFIGURATION');
END;

CREATE TRIGGER IF NOT EXISTS run_configurations_no_delete
BEFORE DELETE ON run_configurations
BEGIN
  SELECT RAISE(ABORT, 'IMMUTABLE_RUN_CONFIGURATION');
END;

CREATE TRIGGER IF NOT EXISTS stages_attempt_insert_guard
BEFORE INSERT ON stages
WHEN NEW.attempt != COALESCE((SELECT MAX(attempt) + 1 FROM stages WHERE run_id=NEW.run_id AND stage_key=NEW.stage_key), 1)
  OR EXISTS (SELECT 1 FROM stages WHERE run_id=NEW.run_id AND stage_key=NEW.stage_key AND state='PENDING')
  OR EXISTS (SELECT 1 FROM stages WHERE run_id=NEW.run_id AND stage_key=NEW.stage_key AND (position != NEW.position OR provider != NEW.provider OR profile != NEW.profile OR model != NEW.model OR effort != NEW.effort))
BEGIN
  SELECT RAISE(ABORT, 'STAGE_ATTEMPT_NOT_APPEND_ONLY');
END;

CREATE TRIGGER IF NOT EXISTS stages_attempt_update_guard
BEFORE UPDATE ON stages
WHEN OLD.state != 'PENDING'
  OR NEW.state NOT IN ('SUCCEEDED', 'FAILED', 'SKIPPED_WITH_REASON')
  OR NEW.id != OLD.id OR NEW.run_id != OLD.run_id OR NEW.stage_key != OLD.stage_key OR NEW.position != OLD.position
  OR NEW.provider != OLD.provider OR NEW.profile != OLD.profile OR NEW.model != OLD.model OR NEW.effort != OLD.effort OR NEW.attempt != OLD.attempt
BEGIN
  SELECT RAISE(ABORT, 'IMMUTABLE_STAGE_ATTEMPT');
END;

CREATE TRIGGER IF NOT EXISTS stages_attempt_no_delete
BEFORE DELETE ON stages
BEGIN
  SELECT RAISE(ABORT, 'IMMUTABLE_STAGE_ATTEMPT');
END;
