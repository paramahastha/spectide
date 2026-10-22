WITH inventory(gate_key,evidence) AS (
  VALUES
    ('analyze_clean','required gate has not run'),
    ('required_tests','required gate has not run'),
    ('converged','required gate has not run'),
    ('final_review','required gate has not run'),
    ('simulated_e2e','required gate has not run'),
    ('workflow_execution','workflow has not reached a terminal outcome')
)
INSERT OR IGNORE INTO quality_gates(run_id,gate_key,verdict,evidence)
SELECT runs.id,inventory.gate_key,'NOT_RUN',inventory.evidence FROM runs CROSS JOIN inventory;

UPDATE quality_gates
SET verdict=(SELECT CASE runs.state WHEN 'COMPLETED' THEN 'PASS' WHEN 'FAILED' THEN 'FAIL' WHEN 'BLOCKED' THEN 'BLOCKED' WHEN 'CANCELLED' THEN 'BLOCKED' ELSE 'NOT_RUN' END FROM runs WHERE runs.id=quality_gates.run_id),
    evidence=(SELECT CASE runs.state WHEN 'COMPLETED' THEN 'workflow reached a successful terminal outcome' WHEN 'FAILED' THEN 'workflow terminated after an execution failure' WHEN 'BLOCKED' THEN 'workflow terminated with blocked or unavailable work' WHEN 'CANCELLED' THEN 'workflow was cancelled before all required work completed' ELSE quality_gates.evidence END FROM runs WHERE runs.id=quality_gates.run_id)
WHERE gate_key='workflow_execution' AND run_id IN (SELECT id FROM runs WHERE state IN ('COMPLETED','FAILED','BLOCKED','CANCELLED'));

INSERT INTO events(run_id,seq,timestamp,stage,type,severity,payload_json)
SELECT runs.id,
       (SELECT COALESCE(MAX(events.seq),0)+1 FROM events WHERE events.run_id=runs.id),
       strftime('%Y-%m-%dT%H:%M:%fZ','now'),NULL,'quality.inventory_initialized','info',
       '{"gate_keys":["analyze_clean","required_tests","converged","final_review","simulated_e2e","workflow_execution"],"initial_verdict":"NOT_RUN"}'
FROM runs
WHERE NOT EXISTS (SELECT 1 FROM events WHERE events.run_id=runs.id AND events.type='quality.inventory_initialized');
