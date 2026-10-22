-- Retention may redact detail from immutable execution histories while keeping
-- their compact identities, outcomes, verdicts, timing, and artifact links.
DROP TRIGGER IF EXISTS execution_attempts_immutable_identity;
CREATE TRIGGER execution_attempts_immutable_identity
BEFORE UPDATE ON execution_attempts
WHEN NOT (
  (
    OLD.result = 'RUNNING' AND NEW.result IN ('SUCCEEDED','FAILED','CANCELLED')
    AND NEW.id = OLD.id AND NEW.run_id = OLD.run_id AND NEW.stage = OLD.stage AND NEW.phase = OLD.phase
    AND NEW.iteration = OLD.iteration AND NEW.provider = OLD.provider AND NEW.profile = OLD.profile
    AND NEW.model = OLD.model AND NEW.effort = OLD.effort AND NEW.started_at = OLD.started_at
    AND NEW.start_event_seq = OLD.start_event_seq
  ) OR (
    OLD.result != 'RUNNING' AND NEW.result = OLD.result
    AND NEW.id = OLD.id AND NEW.run_id = OLD.run_id AND NEW.stage = OLD.stage AND NEW.phase = OLD.phase
    AND NEW.iteration = OLD.iteration AND NEW.provider = OLD.provider AND NEW.profile = OLD.profile
    AND NEW.model = OLD.model AND NEW.effort = OLD.effort AND NEW.started_at = OLD.started_at
    AND NEW.finished_at IS OLD.finished_at AND NEW.start_event_seq = OLD.start_event_seq
    AND NEW.completion_event_seq = OLD.completion_event_seq AND NEW.process_id = OLD.process_id
    AND NEW.duration_ms = OLD.duration_ms AND NEW.exit_code = OLD.exit_code
    AND NEW.error = '' AND NEW.stdout_sha256 = OLD.stdout_sha256 AND NEW.stderr_sha256 = OLD.stderr_sha256
    AND NEW.evidence_json = '{}' AND NEW.artifact_ids_json = OLD.artifact_ids_json
    AND (OLD.error != '' OR OLD.evidence_json != '{}')
  )
)
BEGIN
  SELECT RAISE(ABORT, 'IMMUTABLE_EXECUTION_ATTEMPT');
END;

DROP TRIGGER IF EXISTS quality_results_no_update;
CREATE TRIGGER quality_results_no_update
BEFORE UPDATE ON quality_results
WHEN NOT (
  NEW.execution_id = OLD.execution_id AND NEW.run_id = OLD.run_id AND NEW.check_key = OLD.check_key
  AND NEW.verdict = OLD.verdict AND NEW.process_id = OLD.process_id AND NEW.exit_code = OLD.exit_code
  AND NEW.duration_ns = OLD.duration_ns AND NEW.started_at = OLD.started_at
  AND NEW.command_json = '[]' AND NEW.summary = ''
  AND (OLD.command_json != '[]' OR OLD.summary != '')
)
BEGIN
  SELECT RAISE(ABORT, 'APPEND_ONLY_QUALITY_RESULT');
END;

CREATE INDEX IF NOT EXISTS execution_attempts_retention ON execution_attempts(finished_at,run_id);
CREATE INDEX IF NOT EXISTS quality_results_retention ON quality_results(started_at,run_id);
