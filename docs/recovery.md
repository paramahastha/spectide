# Run recovery and checkpoints

Run transitions and their corresponding events are committed atomically. Each completed simulated stage stores an append-only cursor checkpoint with the completed stage, next stage, immutable run-configuration hash, and canonical workspace identity hash. Event cursors remain monotonic within each run.

The cursor's workspace hash identifies the canonical path; it does not fingerprint repository contents. A fresh owner may pause, resume, or cancel an active run at its safe stage boundary. A missing or expired owner lease causes running or paused work to become `BLOCKED`, appends a `run.interrupted` event with `WORKSPACE_DRIFT`, and releases its abandoned workspace lock.

Restart resume after an expired owner lease is deliberately denied. It requires matching canonical workspace identity, immutable configuration, checkpoint identity, checkpointed artifact path and content hashes, Git HEAD, and managed-path hashes; those checks are not yet implemented. Preserve the run report and use a new run after reviewing the repository. SpecTide does not reset or overwrite user changes.

The storage layer exposes private database backup and restore operations; dedicated CLI backup/restore commands are not yet available. These operations reject overwrite, check database integrity and schema compatibility, and use private file permissions. Retention expires detailed event/checkpoint payloads after 90 days and caps detailed history at 1,000 runs by pruning the oldest terminal details first. Active runs are not pruned.
