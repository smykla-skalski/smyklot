-- SQLite twin: 050_config_file_workload.sql

ALTER TABLE queue_policies DROP CONSTRAINT queue_policies_kind_check;
ALTER TABLE queue_policies ADD CONSTRAINT queue_policies_kind_check CHECK (kind IN (
    'webhook_delivery', 'pending_ci', 'pending_ci_gate', 'catalog_refresh',
    'reaction_scan', 'config_migration', 'config_file_sync', 'sync_scan',
    'sync_apply', 'path_refresh', 'delivery_cleanup', 'auth_cleanup'
));

ALTER TABLE queue_items DROP CONSTRAINT queue_items_kind_check;
ALTER TABLE queue_items ADD CONSTRAINT queue_items_kind_check CHECK (kind IN (
    'webhook_delivery', 'pending_ci', 'pending_ci_gate', 'catalog_refresh',
    'reaction_scan', 'config_migration', 'config_file_sync', 'sync_scan',
    'sync_apply', 'path_refresh', 'delivery_cleanup', 'auth_cleanup', 'schedule_change'
));


INSERT INTO queue_policies (kind, scope_id, enabled, cadence_seconds, profile_id,
    default_priority, retry_delay_seconds, retention_seconds, configuration, updated_at)
VALUES ('config_file_sync', 'root', TRUE, 900, 'always-open',
    'normal', 30, 172800, '{}', '1970-01-01T00:00:00.000000000Z');
