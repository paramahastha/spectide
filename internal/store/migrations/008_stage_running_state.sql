DROP TRIGGER IF EXISTS stages_attempt_insert_guard;
CREATE TRIGGER stages_attempt_insert_guard
BEFORE INSERT ON stages
WHEN NEW.attempt != COALESCE((SELECT MAX(attempt) + 1 FROM stages WHERE run_id=NEW.run_id AND stage_key=NEW.stage_key), 1)
  OR EXISTS (SELECT 1 FROM stages WHERE run_id=NEW.run_id AND stage_key=NEW.stage_key AND state IN ('PENDING', 'RUNNING'))
  OR EXISTS (SELECT 1 FROM stages WHERE run_id=NEW.run_id AND stage_key=NEW.stage_key AND (position != NEW.position OR provider != NEW.provider OR profile != NEW.profile OR model != NEW.model OR effort != NEW.effort))
BEGIN
  SELECT RAISE(ABORT, 'STAGE_ATTEMPT_NOT_APPEND_ONLY');
END;

DROP TRIGGER IF EXISTS stages_attempt_update_guard;
CREATE TRIGGER stages_attempt_update_guard
BEFORE UPDATE ON stages
WHEN NOT (
  (OLD.state = 'PENDING' AND NEW.state IN ('RUNNING', 'SUCCEEDED', 'FAILED', 'SKIPPED_WITH_REASON'))
  OR (OLD.state = 'RUNNING' AND NEW.state IN ('SUCCEEDED', 'FAILED', 'SKIPPED_WITH_REASON'))
)
  OR NEW.id != OLD.id OR NEW.run_id != OLD.run_id OR NEW.stage_key != OLD.stage_key OR NEW.position != OLD.position
  OR NEW.provider != OLD.provider OR NEW.profile != OLD.profile OR NEW.model != OLD.model OR NEW.effort != OLD.effort OR NEW.attempt != OLD.attempt
BEGIN
  SELECT RAISE(ABORT, 'IMMUTABLE_STAGE_ATTEMPT');
END;
