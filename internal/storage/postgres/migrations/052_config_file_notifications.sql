-- Changes survive a stopped service and are consumed with queue scheduling.
-- The connection revision belongs to comparisons, not wake-up notifications.
CREATE TABLE config_file_notifications (
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    scope_key TEXT NOT NULL,
    repository_id TEXT REFERENCES repositories(id) ON DELETE CASCADE,
    requested_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (target_id, scope_key),
    CHECK (
        (scope_key = 'workspace' AND repository_id IS NULL) OR
        (scope_key = 'repository:' || repository_id AND repository_id IS NOT NULL)
    )
);
