-- Add configuration connections without losing queued work or event history.
-- The migration runner wraps the entire rebuild in one transaction.

CREATE TABLE queue_policies_next (
    kind TEXT NOT NULL CHECK (kind IN (
        'webhook_delivery', 'pending_ci', 'pending_ci_gate', 'catalog_refresh', 'reaction_scan',
        'config_migration', 'config_file_sync', 'sync_scan', 'sync_apply', 'path_refresh',
        'delivery_cleanup', 'auth_cleanup'
    )),
    scope_id TEXT NOT NULL,
    target_id TEXT REFERENCES targets(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    cadence_seconds INTEGER NOT NULL CHECK (cadence_seconds >= 0),
    profile_id TEXT NOT NULL REFERENCES schedule_profiles(id),
    default_priority TEXT NOT NULL CHECK (default_priority IN ('low', 'normal', 'high', 'urgent')),
    retry_delay_seconds INTEGER NOT NULL CHECK (retry_delay_seconds >= 0),
    retention_seconds INTEGER CHECK (retention_seconds >= 0),
    approval_ttl_seconds INTEGER CHECK (approval_ttl_seconds > 0),
    configuration TEXT NOT NULL DEFAULT '{}',
    revision INTEGER NOT NULL DEFAULT 1,
    updated_by TEXT REFERENCES accounts(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (kind, scope_id),
    CHECK ((scope_id = 'root' AND target_id IS NULL) OR scope_id = target_id)
);


INSERT INTO queue_policies_next SELECT * FROM queue_policies;
DROP TABLE queue_policies;
ALTER TABLE queue_policies_next RENAME TO queue_policies;

CREATE INDEX queue_policies_target_idx ON queue_policies (target_id, kind);


CREATE TABLE queue_items_next (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN (
        'webhook_delivery', 'pending_ci', 'pending_ci_gate', 'catalog_refresh', 'reaction_scan',
        'config_migration', 'config_file_sync', 'sync_scan', 'sync_apply', 'path_refresh',
        'delivery_cleanup', 'auth_cleanup', 'schedule_change'
    )),
    lane TEXT NOT NULL CHECK (lane IN ('webhook', 'pending_ci', 'maintenance')),
    target_id TEXT,
    repository_id TEXT,
    source_kind TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN (
        'awaiting_approval', 'scheduled', 'blocked', 'ready', 'running',
        'retrying', 'succeeded', 'failed', 'cancelled', 'superseded'
    )),
    priority TEXT NOT NULL CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    priority_overridden INTEGER NOT NULL DEFAULT 0 CHECK (priority_overridden IN (0, 1)),
    window_mode TEXT NOT NULL CHECK (window_mode IN ('respect', 'bypass')),
    immediate_dispatch INTEGER NOT NULL DEFAULT 0 CHECK (immediate_dispatch IN (0, 1)),
    profile_id TEXT REFERENCES schedule_profiles(id),
    not_before TEXT NOT NULL,
    cadence_anchor_at TEXT,
    eligible_at TEXT NOT NULL,
    estimated_start_at TEXT,
    blocked_reason TEXT NOT NULL DEFAULT '',
    progress_current INTEGER NOT NULL DEFAULT 0 CHECK (progress_current >= 0),
    progress_total INTEGER NOT NULL DEFAULT 0 CHECK (progress_total >= 0),
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    lease_expires_at TEXT,
    requested_by TEXT REFERENCES accounts(id),
    reason TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '{}',
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT
);


INSERT INTO queue_items_next SELECT * FROM queue_items;

CREATE TEMP TABLE config_sync_event_sequence AS
SELECT seq FROM sqlite_sequence WHERE name = 'queue_events';

CREATE TABLE queue_events_next (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    queue_item_id TEXT NOT NULL REFERENCES queue_items_next(id) ON DELETE CASCADE,
    actor_account_id TEXT REFERENCES accounts(id),
    kind TEXT NOT NULL,
    state TEXT NOT NULL,
    summary TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);


INSERT INTO queue_events_next SELECT * FROM queue_events;
DROP TABLE queue_events;
DROP TABLE queue_items;
ALTER TABLE queue_items_next RENAME TO queue_items;
ALTER TABLE queue_events_next RENAME TO queue_events;

-- Pruned events must never let the next event reuse an older ID.
UPDATE sqlite_sequence SET seq = MAX(seq, COALESCE((SELECT seq FROM config_sync_event_sequence), 0))
WHERE name = 'queue_events';
DROP TABLE config_sync_event_sequence;

CREATE UNIQUE INDEX queue_items_source_idx
ON queue_items (source_kind, source_id)
WHERE source_kind <> '' AND source_id <> ''
  AND state NOT IN ('succeeded', 'failed', 'cancelled', 'superseded');

CREATE INDEX queue_items_ready_idx
ON queue_items (lane, state, immediate_dispatch DESC, priority, eligible_at, created_at, id)
WHERE state IN ('scheduled', 'ready', 'retrying');

CREATE INDEX queue_items_target_idx
ON queue_items (target_id, state, updated_at DESC, id);


CREATE INDEX queue_events_item_idx ON queue_events (queue_item_id, id);


CREATE INDEX queue_items_page_idx
ON queue_items (
    (CASE WHEN finished_at IS NULL THEN 0 ELSE 1 END),
    (CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END),
    eligible_at,
    updated_at DESC,
    id,
    finished_at
);

CREATE INDEX queue_items_source_history_idx
ON queue_items (source_kind, source_id);

CREATE INDEX queue_items_source_open_idx
ON queue_items (source_kind, id)
WHERE state NOT IN ('succeeded', 'failed', 'cancelled', 'superseded');

CREATE INDEX queue_items_lane_finished_idx
ON queue_items (lane, finished_at DESC)
WHERE started_at IS NOT NULL AND finished_at IS NOT NULL;


INSERT INTO queue_policies (kind, scope_id, enabled, cadence_seconds, profile_id,
    default_priority, retry_delay_seconds, retention_seconds, configuration, updated_at)
VALUES ('config_file_sync', 'root', 1, 900, 'always-open',
    'normal', 30, 172800, '{}', '1970-01-01T00:00:00.000000000Z');
