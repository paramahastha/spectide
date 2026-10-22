CREATE TABLE IF NOT EXISTS profile_capabilities (
  profile_name TEXT PRIMARY KEY REFERENCES profiles(name) ON DELETE CASCADE,
  capability_json TEXT NOT NULL,
  observed_at TEXT NOT NULL
);
