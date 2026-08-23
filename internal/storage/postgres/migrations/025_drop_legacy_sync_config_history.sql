-- Remove the superseded Sync-only checkpoint schema. The SQLite twin is
-- 040_drop_legacy_sync_config_history.sql, and TestSchemaParity fails if they drift.

-- Root audit rows remain useful history after their source snapshots go away.
-- Clear only the dangling legacy pointer and retain the event itself.
UPDATE app_audit_events
SET source_kind = NULL, source_id = NULL
WHERE source_kind = 'sync_config_checkpoint';

DROP INDEX audit_sync_config_checkpoint_idx;

ALTER TABLE audit_entries
    DROP COLUMN sync_config_checkpoint_id;

DROP TABLE sync_config_checkpoint_items;
DROP TABLE sync_config_checkpoints;
