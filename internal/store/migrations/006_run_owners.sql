CREATE TABLE IF NOT EXISTS run_owners (
  run_id TEXT PRIMARY KEY REFERENCES runs(id),
  owner_id TEXT NOT NULL,
  heartbeat_at TEXT NOT NULL
);
