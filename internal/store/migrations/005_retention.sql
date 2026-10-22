ALTER TABLE runs ADD COLUMN details_pruned_at TEXT;
CREATE INDEX IF NOT EXISTS runs_detail_retention ON runs(details_pruned_at, state, created_at);
