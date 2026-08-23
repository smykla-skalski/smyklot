-- Replace mixed sparse checkpoint history with complete point-in-time
-- snapshots. Audit text remains available, while links to discarded snapshots
-- are cleared before startup creates fresh baselines.

UPDATE audit_entries
SET settings_checkpoint_id = NULL
WHERE settings_checkpoint_id IS NOT NULL;

UPDATE app_audit_events
SET source_kind = NULL, source_id = NULL
WHERE source_kind = 'settings_checkpoint';

DELETE FROM settings_checkpoint_items;
DELETE FROM settings_checkpoints;

ALTER TABLE settings_checkpoints
    ADD COLUMN restored_side TEXT CHECK (restored_side IN ('before', 'after'));

DROP TABLE settings_checkpoint_items;

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
    CHECK (before_document IS NOT NULL OR after_document IS NOT NULL)
);
