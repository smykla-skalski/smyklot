-- Every path a repository is known to hold, so the panel can offer them.
-- See the SQLite migration of the same name for why it is one row per
-- repository, and for why the newline-joined text it was written with did not
-- survive: migration 021 empties the table and it holds JSON from there on.
CREATE TABLE sync_repository_paths (
    repository_id TEXT PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    paths TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX sync_repository_paths_target_idx
ON sync_repository_paths (target_id, repository_id);
