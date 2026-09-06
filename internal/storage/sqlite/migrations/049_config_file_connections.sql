-- Settings keep the opt-in under the same revision, audit and restore contract
-- as every other panel control. Background observations have their own revision.
ALTER TABLE targets ADD COLUMN config_file_sync_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE repositories ADD COLUMN config_file_sync_enabled BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE config_file_connections (
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    scope_key TEXT NOT NULL,
    repository_id TEXT REFERENCES repositories(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL CHECK (revision > 0),
    document TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (target_id, scope_key),
    CHECK (
        (scope_key = 'workspace' AND repository_id IS NULL) OR
        (scope_key = 'repository:' || repository_id AND repository_id IS NOT NULL)
    )
);
