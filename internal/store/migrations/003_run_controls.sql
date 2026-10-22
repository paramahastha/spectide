CREATE TABLE IF NOT EXISTS run_controls (
  run_id TEXT PRIMARY KEY REFERENCES runs(id),
  action TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
