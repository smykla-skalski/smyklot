-- Shared background-work queue and scheduling policy.

CREATE TABLE schedule_profiles (
    id TEXT PRIMARY KEY,
    target_id TEXT REFERENCES targets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    timezone TEXT NOT NULL,
    system INTEGER NOT NULL DEFAULT 0 CHECK (system IN (0, 1)),
    archived_at TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    created_by TEXT REFERENCES accounts(id),
    updated_by TEXT REFERENCES accounts(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX schedule_profiles_name_idx
ON schedule_profiles (COALESCE(target_id, ''), lower(name));

CREATE TABLE schedule_windows (
    profile_id TEXT NOT NULL REFERENCES schedule_profiles(id) ON DELETE CASCADE,
    weekday INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_minute INTEGER NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute INTEGER NOT NULL CHECK (end_minute BETWEEN 1 AND 1440),
    PRIMARY KEY (profile_id, weekday, start_minute, end_minute),
    CHECK (start_minute < end_minute)
);

CREATE TABLE schedule_exceptions (
    profile_id TEXT NOT NULL REFERENCES schedule_profiles(id) ON DELETE CASCADE,
    local_date TEXT NOT NULL,
    closed INTEGER NOT NULL DEFAULT 0 CHECK (closed IN (0, 1)),
    start_minute INTEGER NOT NULL DEFAULT 0 CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute INTEGER NOT NULL DEFAULT 0 CHECK (end_minute BETWEEN 0 AND 1440),
    PRIMARY KEY (profile_id, local_date, start_minute, end_minute),
    CHECK ((closed = 1 AND start_minute = 0 AND end_minute = 0)
        OR (closed = 0 AND start_minute < end_minute))
);

CREATE TABLE queue_policies (
    kind TEXT NOT NULL CHECK (kind IN (
        'webhook_delivery', 'pending_ci', 'pending_ci_gate', 'catalog_refresh', 'reaction_scan',
        'config_migration', 'sync_scan', 'sync_apply', 'path_refresh',
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

CREATE INDEX queue_policies_target_idx ON queue_policies (target_id, kind);

CREATE TABLE queue_items (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN (
        'webhook_delivery', 'pending_ci', 'pending_ci_gate', 'catalog_refresh', 'reaction_scan',
        'config_migration', 'sync_scan', 'sync_apply', 'path_refresh',
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

CREATE UNIQUE INDEX queue_items_source_idx
ON queue_items (source_kind, source_id)
WHERE source_kind <> '' AND source_id <> ''
  AND state NOT IN ('succeeded', 'failed', 'cancelled', 'superseded');

CREATE INDEX queue_items_ready_idx
ON queue_items (lane, state, immediate_dispatch DESC, priority, eligible_at, created_at, id)
WHERE state IN ('scheduled', 'ready', 'retrying');

CREATE INDEX queue_items_target_idx
ON queue_items (target_id, state, updated_at DESC, id);

CREATE TABLE queue_dispatch_state (
    lane TEXT PRIMARY KEY CHECK (lane IN ('webhook', 'pending_ci', 'maintenance')),
    priority_cursor INTEGER NOT NULL DEFAULT 0 CHECK (priority_cursor BETWEEN 0 AND 14),
    target_cursor TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);

INSERT INTO queue_dispatch_state (lane, priority_cursor, target_cursor, updated_at) VALUES
    ('webhook', 0, '', '1970-01-01T00:00:00.000000000Z'),
    ('pending_ci', 0, '', '1970-01-01T00:00:00.000000000Z'),
    ('maintenance', 0, '', '1970-01-01T00:00:00.000000000Z');

CREATE TABLE queue_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    queue_item_id TEXT NOT NULL REFERENCES queue_items(id) ON DELETE CASCADE,
    actor_account_id TEXT REFERENCES accounts(id),
    kind TEXT NOT NULL,
    state TEXT NOT NULL,
    summary TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE INDEX queue_events_item_idx ON queue_events (queue_item_id, id);

CREATE TABLE schedule_requests (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'approved', 'rejected', 'withdrawn', 'stale')),
    base_revision INTEGER NOT NULL,
    base_target_id TEXT REFERENCES targets(id) ON DELETE CASCADE,
    profile_id TEXT REFERENCES schedule_profiles(id),
    custom_profile TEXT,
    cadence_seconds INTEGER NOT NULL CHECK (cadence_seconds >= 0),
    default_priority TEXT NOT NULL CHECK (default_priority IN ('low', 'normal', 'high', 'urgent')),
    configuration TEXT NOT NULL DEFAULT '{}',
    reason TEXT NOT NULL,
    requested_by TEXT NOT NULL REFERENCES accounts(id),
    reviewed_by TEXT REFERENCES accounts(id),
    decision_reason TEXT NOT NULL DEFAULT '',
    promoted_profile_id TEXT REFERENCES schedule_profiles(id),
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    reviewed_at TEXT
);

CREATE INDEX schedule_requests_target_idx
ON schedule_requests (target_id, state, created_at DESC);
CREATE INDEX schedule_requests_state_idx
ON schedule_requests (state, created_at);
CREATE UNIQUE INDEX schedule_requests_pending_idx
ON schedule_requests (target_id, kind) WHERE state = 'pending';

INSERT INTO schedule_profiles (
    id, name, timezone, system, revision, created_at, updated_at
) VALUES (
    'always-open', 'Always Open', 'UTC', 1, 1,
    '1970-01-01T00:00:00.000000000Z', '1970-01-01T00:00:00.000000000Z'
);

INSERT INTO schedule_windows (profile_id, weekday, start_minute, end_minute) VALUES
    ('always-open', 0, 0, 1440), ('always-open', 1, 0, 1440),
    ('always-open', 2, 0, 1440), ('always-open', 3, 0, 1440),
    ('always-open', 4, 0, 1440), ('always-open', 5, 0, 1440),
    ('always-open', 6, 0, 1440);

INSERT INTO queue_policies (
    kind, scope_id, enabled, cadence_seconds, profile_id, default_priority,
    retry_delay_seconds, retention_seconds, approval_ttl_seconds, configuration, updated_at
) VALUES
    ('webhook_delivery', 'root', 1, 0, 'always-open', 'urgent', 2, 2592000, NULL, '{"max_delay_seconds":300,"max_attempts":8}', '1970-01-01T00:00:00.000000000Z'),
    ('pending_ci', 'root', 1, 300, 'always-open', 'normal', 5, NULL, NULL, '{"active_check_seconds":300,"no_check_grace_seconds":600,"defer_after_seconds":3600,"deferred_check_seconds":21600,"passing_quiet_seconds":30}', '1970-01-01T00:00:00.000000000Z'),
    ('pending_ci_gate', 'root', 1, 300, 'always-open', 'normal', 30, NULL, NULL, '{}', '1970-01-01T00:00:00.000000000Z'),
    ('catalog_refresh', 'root', 1, 300, 'always-open', 'normal', 30, NULL, NULL, '{}', '1970-01-01T00:00:00.000000000Z'),
    ('reaction_scan', 'root', 1, 300, 'always-open', 'normal', 30, NULL, NULL, '{}', '1970-01-01T00:00:00.000000000Z'),
    ('config_migration', 'root', 1, 300, 'always-open', 'normal', 30, NULL, NULL, '{}', '1970-01-01T00:00:00.000000000Z'),
    ('sync_scan', 'root', 1, 21600, 'always-open', 'normal', 300, NULL, 7200, '{}', '1970-01-01T00:00:00.000000000Z'),
    ('sync_apply', 'root', 1, 0, 'always-open', 'normal', 300, NULL, NULL, '{}', '1970-01-01T00:00:00.000000000Z'),
    ('path_refresh', 'root', 1, 3600, 'always-open', 'low', 300, NULL, NULL, '{}', '1970-01-01T00:00:00.000000000Z'),
    ('delivery_cleanup', 'root', 1, 300, 'always-open', 'low', 300, 2592000, NULL, '{}', '1970-01-01T00:00:00.000000000Z'),
    ('auth_cleanup', 'root', 1, 300, 'always-open', 'low', 300, NULL, NULL, '{}', '1970-01-01T00:00:00.000000000Z');

INSERT INTO queue_items (
    id, kind, lane, target_id, repository_id, source_kind, source_id, title, summary,
    state, priority, window_mode, immediate_dispatch, not_before, eligible_at,
    attempt, created_at, updated_at, finished_at
)
SELECT
    'delivery:' || CAST(id AS TEXT), 'webhook_delivery', 'webhook', target_id, repository_id,
    'delivery', CAST(id AS TEXT), 'Webhook: ' || event, repository_full_name,
    CASE status WHEN 'succeeded' THEN 'succeeded' WHEN 'failed' THEN 'failed'
        ELSE CASE WHEN attempt_count > 0 THEN 'retrying' ELSE 'scheduled' END END,
    'urgent', 'bypass', 0, COALESCE(next_attempt_at, claimed_at),
    COALESCE(next_attempt_at, claimed_at), attempt_count, claimed_at,
    COALESCE(finished_at, claimed_at), finished_at
FROM deliveries;

INSERT INTO queue_items (
    id, kind, lane, target_id, repository_id, source_kind, source_id, title, summary,
    state, priority, window_mode, immediate_dispatch, profile_id, not_before, eligible_at,
    attempt, created_at, updated_at, finished_at
)
SELECT
    'pending-ci:' || CAST(id AS TEXT), 'pending_ci', 'pending_ci', target_id, repository_id,
    'pending_ci', CAST(id AS TEXT),
    'Pending CI ' || repository_full_name || ' #' || CAST(pull_request AS TEXT), reason,
    CASE WHEN cleanup_pending = 1 THEN 'retrying'
        WHEN lifecycle = 'armed' THEN 'scheduled'
        WHEN lifecycle = 'merged' THEN 'succeeded'
        WHEN lifecycle = 'cancelled' THEN 'cancelled' ELSE 'superseded' END,
    'normal', 'respect', 0, 'always-open', next_check_at, next_check_at,
    cleanup_attempts, requested_at, updated_at,
    CASE WHEN cleanup_pending = 1 THEN NULL ELSE finished_at END
FROM pending_ci_requests;

INSERT INTO queue_items (
    id, kind, lane, target_id, source_kind, source_id, title, summary, state, priority,
    window_mode, immediate_dispatch, profile_id, not_before, eligible_at, attempt,
    progress_current, progress_total, created_at, updated_at, started_at, finished_at
)
SELECT
    'sync-plan:' || id, 'sync_apply', 'maintenance', target_id, 'sync_plan', id,
    'Organization sync',
    CAST(create_count AS TEXT) || ' to add, ' || CAST(update_count AS TEXT) ||
        ' to change, ' || CAST(delete_count AS TEXT) || ' to remove',
    CASE state WHEN 'computed' THEN 'awaiting_approval' WHEN 'approved' THEN 'scheduled'
        WHEN 'applying' THEN 'running' WHEN 'applied' THEN 'succeeded'
        WHEN 'failed' THEN 'failed' WHEN 'discarded' THEN 'cancelled' ELSE 'superseded' END,
    'normal', 'respect', 0, 'always-open', COALESCE(approved_at, computed_at),
    COALESCE(approved_at, computed_at), attempt,
    (SELECT COUNT(*) FROM sync_plan_actions
        WHERE plan_id = sync_plans.id AND state <> 'pending'),
    (SELECT COUNT(*) FROM sync_plan_actions WHERE plan_id = sync_plans.id),
    computed_at,
    COALESCE(finished_at, approved_at, computed_at), started_at, finished_at
FROM sync_plans;

INSERT INTO queue_events (
    queue_item_id, kind, state, summary, details, created_at
)
SELECT id, 'backfilled', state, 'Migrated existing durable work', '{}', created_at
FROM queue_items;
