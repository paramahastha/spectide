CREATE TABLE IF NOT EXISTS workspace_metadata (
  singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
  workspace_id TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);
ALTER TABLE runs ADD COLUMN project_id TEXT REFERENCES projects(id);
UPDATE runs
SET project_id = (
  SELECT p.id FROM projects p WHERE p.canonical_path = runs.project
)
WHERE project_id IS NULL AND EXISTS (
  SELECT 1 FROM projects p WHERE p.canonical_path = runs.project
);
CREATE INDEX IF NOT EXISTS runs_project_history ON runs(project_id, created_at DESC, id DESC) WHERE project_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS workspace_imports (
  id TEXT PRIMARY KEY,
  source_workspace_id TEXT NOT NULL,
  destination_workspace_id TEXT NOT NULL,
  status TEXT NOT NULL,
  backup_reference TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
