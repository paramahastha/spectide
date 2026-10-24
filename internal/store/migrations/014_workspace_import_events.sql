CREATE TABLE IF NOT EXISTS workspace_import_events (
  id TEXT PRIMARY KEY,
  import_id TEXT NOT NULL REFERENCES workspace_imports(id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL CHECK(sequence > 0),
  status TEXT NOT NULL CHECK(status IN ('DETECTED','AWAITING_CONFIRMATION','BACKED_UP','RESTORED','VALIDATED','COMPLETED')),
  occurred_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS workspace_import_events_by_import
  ON workspace_import_events(import_id, sequence);
