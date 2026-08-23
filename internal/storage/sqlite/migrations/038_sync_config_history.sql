-- Restorable installation sync configuration. The PostgreSQL twin is
-- 023_sync_config_history.sql.

CREATE TABLE sync_config_checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL CHECK (action IN ('baseline', 'save', 'restore')),
    restored_from_id INTEGER REFERENCES sync_config_checkpoints(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX sync_config_checkpoints_target_idx
    ON sync_config_checkpoints (target_id, id DESC);

CREATE TABLE sync_config_checkpoint_items (
    checkpoint_id INTEGER NOT NULL
        REFERENCES sync_config_checkpoints(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    enabled INTEGER NOT NULL,
    document TEXT NOT NULL,
    digest TEXT NOT NULL,
    revision INTEGER NOT NULL,
    PRIMARY KEY (checkpoint_id, kind)
);

ALTER TABLE audit_entries
    ADD COLUMN sync_config_checkpoint_id INTEGER
        REFERENCES sync_config_checkpoints(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX audit_sync_config_checkpoint_idx
    ON audit_entries (sync_config_checkpoint_id)
    WHERE sync_config_checkpoint_id IS NOT NULL;

-- Capture every installation's state at upgrade time without claiming that
-- the upgrade itself was a user change. Empty installations need a checkpoint
-- too: otherwise their first saved checkpoint has no historical baseline.
INSERT INTO sync_config_checkpoints (
    target_id, actor_account_id, action, created_at
)
SELECT
    target.id,
    COALESCE((
        SELECT latest.updated_by
        FROM sync_configs latest
        WHERE latest.target_id = target.id
        ORDER BY latest.updated_at DESC, latest.kind
        LIMIT 1
    ), target.account_id),
    'baseline',
    COALESCE((
        SELECT MAX(latest.updated_at)
        FROM sync_configs latest
        WHERE latest.target_id = target.id
    ), target.settings_updated_at)
FROM targets target;

INSERT INTO sync_config_checkpoint_items (
    checkpoint_id, kind, enabled, document, digest, revision
)
SELECT checkpoint.id, config.kind, config.enabled, config.document, config.digest, config.revision
FROM sync_config_checkpoints checkpoint
JOIN sync_configs config ON config.target_id = checkpoint.target_id
WHERE checkpoint.action = 'baseline';
