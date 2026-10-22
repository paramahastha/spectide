ALTER TABLE quality_gates ADD COLUMN required INTEGER NOT NULL DEFAULT 1;
ALTER TABLE quality_gates ADD COLUMN applicable INTEGER NOT NULL DEFAULT 1;

UPDATE quality_gates
SET required=0, applicable=0
WHERE gate_key='simulated_e2e'
  AND run_id IN (
    SELECT DISTINCT c.run_id
    FROM run_configurations AS c,
         json_each(c.config_json, '$.stages') AS stage
    WHERE json_extract(stage.value, '$.decision.route.provider') != 'fake'
  );
