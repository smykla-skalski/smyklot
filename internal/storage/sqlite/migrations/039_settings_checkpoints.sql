-- Sparse, immutable checkpoints for the bounded settings resources. The
-- PostgreSQL twin is 024_settings_checkpoints.sql. Legacy Sync checkpoints stay
-- untouched so their audit links and restore semantics remain compatible.

CREATE TABLE settings_checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scope TEXT NOT NULL CHECK (scope IN ('root', 'installation')),
    target_id TEXT REFERENCES targets(id) ON DELETE CASCADE,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL CHECK (action IN ('baseline', 'save', 'restore')),
    restored_from_id INTEGER REFERENCES settings_checkpoints(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    CHECK (
        (scope = 'root' AND target_id IS NULL) OR
        (scope = 'installation' AND target_id IS NOT NULL)
    ),
    CHECK (restored_from_id IS NULL OR action = 'restore')
);

CREATE INDEX settings_checkpoints_scope_target_idx
    ON settings_checkpoints (scope, target_id, id DESC);

-- Baselines describe the complete state first observed by generic history, so
-- later restores can reason about resources that did not exist. Partial
-- uniqueness makes both upgrade backfill and first reconciliation safe to
-- retry without conflating them with sparse Save and Restore history.
CREATE UNIQUE INDEX settings_checkpoints_installation_baseline_idx
    ON settings_checkpoints (target_id)
    WHERE scope = 'installation' AND action = 'baseline';

CREATE UNIQUE INDEX settings_checkpoints_root_baseline_idx
    ON settings_checkpoints (scope)
    WHERE scope = 'root' AND action = 'baseline';

CREATE TABLE settings_checkpoint_items (
    checkpoint_id INTEGER NOT NULL
        REFERENCES settings_checkpoints(id) ON DELETE CASCADE,
    item_kind TEXT NOT NULL CHECK (
        item_kind IN ('target', 'repository', 'sync_config', 'sync_override', 'runtime')
    ),
    repository_id TEXT NOT NULL DEFAULT '',
    repository_full_name TEXT NOT NULL DEFAULT '',
    sync_kind TEXT NOT NULL DEFAULT '' CHECK (
        sync_kind IN ('', 'labels', 'settings', 'rulesets', 'files')
    ),
    document_version INTEGER NOT NULL CHECK (document_version > 0),
    before_document TEXT,
    before_revision INTEGER,
    before_digest TEXT,
    after_document TEXT,
    after_revision INTEGER,
    after_digest TEXT,
    PRIMARY KEY (checkpoint_id, item_kind, repository_id, sync_kind),
    CHECK (
        (item_kind IN ('target', 'runtime') AND
            repository_id = '' AND repository_full_name = '' AND sync_kind = '') OR
        (item_kind = 'repository' AND
            repository_id <> '' AND repository_full_name <> '' AND sync_kind = '') OR
        (item_kind = 'sync_config' AND
            repository_id = '' AND repository_full_name = '' AND sync_kind <> '') OR
        (item_kind = 'sync_override' AND
            repository_id <> '' AND repository_full_name <> '' AND sync_kind <> '')
    ),
    CHECK (
        (before_document IS NULL AND before_revision IS NULL AND before_digest IS NULL) OR
        (before_document IS NOT NULL AND before_revision IS NOT NULL AND before_revision >= 0 AND
            before_digest IS NOT NULL AND before_digest <> '')
    ),
    CHECK (
        (after_document IS NULL AND after_revision IS NULL AND after_digest IS NULL) OR
        (after_document IS NOT NULL AND after_revision IS NOT NULL AND after_revision >= 0 AND
            after_digest IS NOT NULL AND after_digest <> '')
    ),
    CHECK (before_document IS NOT NULL OR after_document IS NOT NULL),
    CHECK (before_digest IS NULL OR after_digest IS NULL OR before_digest <> after_digest)
);

ALTER TABLE audit_entries
    ADD COLUMN settings_checkpoint_id INTEGER
        REFERENCES settings_checkpoints(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX audit_settings_checkpoint_idx
    ON audit_entries (settings_checkpoint_id)
    WHERE settings_checkpoint_id IS NOT NULL;
