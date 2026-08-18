-- Zero is a valid quiet period, but still requires two observations.
ALTER TABLE runtime_settings RENAME TO runtime_settings_old;

CREATE TABLE runtime_settings (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    bot_config TEXT,
    log_level TEXT CHECK (
        log_level IS NULL OR log_level IN ('debug', 'info', 'warn', 'error')
    ),
    session_ttl_seconds INTEGER CHECK (
        session_ttl_seconds IS NULL OR session_ttl_seconds >= 60
    ),
    poll_interval_seconds INTEGER CHECK (
        poll_interval_seconds IS NULL OR poll_interval_seconds >= 0
    ),
    pending_ci_quiet_period_seconds INTEGER CHECK (
        pending_ci_quiet_period_seconds IS NULL OR pending_ci_quiet_period_seconds >= 0
    ),
    revision INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by_account_id TEXT NOT NULL REFERENCES accounts(id)
);

INSERT INTO runtime_settings (
    singleton, bot_config, log_level, session_ttl_seconds,
    poll_interval_seconds, pending_ci_quiet_period_seconds,
    revision, updated_at, updated_by_account_id
)
SELECT
    singleton, bot_config, log_level, session_ttl_seconds,
    poll_interval_seconds, pending_ci_quiet_period_seconds,
    revision, updated_at, updated_by_account_id
FROM runtime_settings_old;

DROP TABLE runtime_settings_old;

-- Panel-owned merge-after-CI defaults and repository overrides.
ALTER TABLE targets
ADD COLUMN pending_ci_mode_default TEXT NOT NULL DEFAULT 'checks'
CHECK (pending_ci_mode_default IN ('labels', 'checks'));

ALTER TABLE targets
ADD COLUMN pending_ci_branch_patterns_default TEXT NOT NULL
DEFAULT '{"include":["~DEFAULT_BRANCH"],"exclude":[]}';

ALTER TABLE targets
ADD COLUMN pending_ci_quiet_period_seconds_override INTEGER
CHECK (
    pending_ci_quiet_period_seconds_override IS NULL OR
    pending_ci_quiet_period_seconds_override BETWEEN 0 AND 86400
);

ALTER TABLE repositories
ADD COLUMN pending_ci_mode_override TEXT
CHECK (pending_ci_mode_override IS NULL OR pending_ci_mode_override IN ('labels', 'checks'));

ALTER TABLE repositories
ADD COLUMN pending_ci_branch_patterns_override TEXT;

ALTER TABLE repositories
ADD COLUMN pending_ci_quiet_period_seconds_override INTEGER
CHECK (
    pending_ci_quiet_period_seconds_override IS NULL OR
    pending_ci_quiet_period_seconds_override BETWEEN 0 AND 86400
);

-- PostgreSQL uses this row as a policy mutex. SQLite already serializes
-- writers, but keeps the same schema and transaction path.
CREATE TABLE pending_ci_policy_lock (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1)
);

INSERT INTO pending_ci_policy_lock (singleton) VALUES (1);

-- The external ruleset transition is durable and independent from repository
-- settings. A saved desire can therefore survive missing GitHub permissions.
CREATE TABLE pending_ci_repository_gates (
    repository_id TEXT PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    desired_mode TEXT NOT NULL CHECK (desired_mode IN ('labels', 'checks')),
    effective_mode TEXT NOT NULL DEFAULT 'none'
        CHECK (effective_mode IN ('none', 'labels', 'checks')),
    readiness TEXT NOT NULL DEFAULT 'provisioning'
        CHECK (readiness IN ('ready', 'provisioning', 'draining', 'blocked')),
    reason TEXT NOT NULL DEFAULT '',
    app_id INTEGER,
    ruleset_id INTEGER,
    ruleset_fingerprint TEXT NOT NULL DEFAULT '',
    generation INTEGER NOT NULL DEFAULT 1,
    observed_at TEXT,
    updated_at TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX pending_ci_repository_gates_target_idx
ON pending_ci_repository_gates (target_id, readiness, repository_id);

INSERT INTO pending_ci_repository_gates (
    repository_id, target_id, desired_mode, effective_mode,
    readiness, reason, updated_at
)
SELECT
    r.id, r.target_id,
    COALESCE(r.pending_ci_mode_override, t.pending_ci_mode_default),
    'none', 'provisioning',
    'Waiting for repository protection reconciliation', r.synced_at
FROM repositories r
JOIN targets t ON t.id = r.target_id;

-- One slot owns Smyklot's stable Check Run context for one commit. The desired
-- and applied digests form an outbox: GitHub writes can be retried after a
-- crash without losing which state the database asked for.
CREATE TABLE pending_ci_check_slots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    installation_id INTEGER NOT NULL,
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    repository_full_name TEXT NOT NULL,
    pull_request INTEGER NOT NULL,
    head_sha TEXT NOT NULL,
    app_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    external_id TEXT NOT NULL,
    generation INTEGER NOT NULL DEFAULT 1,
    check_run_id INTEGER,
    check_url TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'provisioning'
        CHECK (state IN ('provisioning', 'ready', 'blocked')),
    desired_status TEXT NOT NULL,
    desired_conclusion TEXT,
    desired_title TEXT NOT NULL,
    desired_summary TEXT NOT NULL,
    desired_actions TEXT NOT NULL DEFAULT '[]',
    desired_digest TEXT NOT NULL,
    applied_digest TEXT NOT NULL DEFAULT '',
    retry_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    UNIQUE (repository_id, head_sha),
    UNIQUE (external_id)
);

CREATE UNIQUE INDEX pending_ci_check_slots_run_idx
ON pending_ci_check_slots (check_run_id)
WHERE check_run_id IS NOT NULL;

CREATE INDEX pending_ci_check_slots_retry_idx
ON pending_ci_check_slots (state, retry_at, id);

-- SQLite cannot drop label's NOT NULL constraint. Rebuild the request and
-- event tables together so the event foreign key and retained history survive.
ALTER TABLE pending_ci_events RENAME TO pending_ci_events_old;

CREATE TABLE pending_ci_requests_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id TEXT NOT NULL,
    installation_id INTEGER NOT NULL,
    repository_id TEXT NOT NULL,
    repository_full_name TEXT NOT NULL,
    pull_request INTEGER NOT NULL,
    head_sha TEXT NOT NULL,
    base_branch TEXT NOT NULL,
    merge_method TEXT NOT NULL CHECK (merge_method IN ('merge', 'squash', 'rebase')),
    required_checks_only INTEGER NOT NULL CHECK (required_checks_only IN (0, 1)),
    requester TEXT NOT NULL,
    source_comment_id INTEGER NOT NULL,
    source_revision TEXT NOT NULL,
    source_sequence INTEGER NOT NULL DEFAULT 1,
    source_order INTEGER NOT NULL DEFAULT 1,
    artifact_kind TEXT NOT NULL DEFAULT 'label'
        CHECK (artifact_kind IN ('label', 'check')),
    label TEXT,
    check_slot_id INTEGER REFERENCES pending_ci_check_slots(id) ON DELETE SET NULL,
    retired_check_slot_id INTEGER REFERENCES pending_ci_check_slots(id) ON DELETE SET NULL,
    authorization_state TEXT NOT NULL DEFAULT 'authorized'
        CHECK (authorization_state IN ('authorized', 'reauthorization_required')),
    gate_state TEXT NOT NULL DEFAULT 'ready'
        CHECK (gate_state IN ('ready', 'readiness_blocked')),
    candidate_head_sha TEXT,
    candidate_base_branch TEXT,
    authorized_by TEXT NOT NULL,
    authorized_at TEXT NOT NULL,
    merge_phase TEXT NOT NULL DEFAULT 'waiting'
        CHECK (merge_phase IN ('waiting', 'claimed', 'check_succeeded')),
    lifecycle TEXT NOT NULL CHECK (lifecycle IN ('armed', 'merged', 'cancelled', 'superseded')),
    schedule TEXT NOT NULL CHECK (schedule IN ('active', 'deferred')),
    next_check_trigger TEXT NOT NULL DEFAULT 'fallback'
        CHECK (next_check_trigger IN ('command', 'webhook', 'fallback', 'quiet_period', 'manual', 'cleanup')),
    next_check_at TEXT NOT NULL,
    lease_expires_at TEXT,
    last_progress_at TEXT NOT NULL,
    last_observed_state TEXT NOT NULL DEFAULT '',
    last_fingerprint TEXT NOT NULL DEFAULT '',
    last_event_key TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    requested_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    finished_at TEXT,
    cleanup_pending INTEGER NOT NULL DEFAULT 0 CHECK (cleanup_pending IN (0, 1)),
    cleanup_artifacts_done INTEGER NOT NULL DEFAULT 0
        CHECK (cleanup_artifacts_done IN (0, 1)),
    cleanup_attempts INTEGER NOT NULL DEFAULT 0,
    cleanup_error TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1,
    CHECK (
        (artifact_kind = 'label' AND label IS NOT NULL AND check_slot_id IS NULL) OR
        (artifact_kind = 'check' AND label IS NULL AND check_slot_id IS NOT NULL)
    )
);

INSERT INTO pending_ci_requests_v2 (
    id, target_id, installation_id, repository_id, repository_full_name,
    pull_request, head_sha, base_branch, merge_method, required_checks_only,
    requester, source_comment_id, source_revision, source_sequence, source_order,
    artifact_kind, label, authorization_state, gate_state,
    authorized_by, authorized_at, merge_phase,
    lifecycle, schedule, next_check_trigger, next_check_at, lease_expires_at,
    last_progress_at, last_observed_state, last_fingerprint, last_event_key,
    reason, requested_at, updated_at, finished_at, cleanup_pending,
    cleanup_artifacts_done, cleanup_attempts, cleanup_error, revision
)
SELECT
    id, target_id, installation_id, repository_id, repository_full_name,
    pull_request, head_sha, base_branch, merge_method, required_checks_only,
    requester, source_comment_id, source_revision, source_sequence, source_order,
    'label', label, 'authorized', 'ready',
    requester, requested_at, 'waiting',
    lifecycle, schedule, next_check_trigger, next_check_at, lease_expires_at,
    last_progress_at, last_observed_state, last_fingerprint, last_event_key,
    reason, requested_at, updated_at, finished_at, cleanup_pending,
    cleanup_artifacts_done, cleanup_attempts, cleanup_error, revision
FROM pending_ci_requests;

CREATE TABLE pending_ci_events_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id INTEGER NOT NULL REFERENCES pending_ci_requests_v2(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN (
        'armed', 'superseded', 'wake_received', 'reconciliation_started',
        'checks_observed', 'merge_started', 'finished', 'cleanup_retry',
        'cleanup_completed'
    )),
    trigger_kind TEXT NOT NULL CHECK (trigger_kind IN (
        'command', 'webhook', 'fallback', 'quiet_period', 'manual', 'cleanup'
    )),
    event_name TEXT NOT NULL DEFAULT '',
    event_key TEXT NOT NULL DEFAULT '',
    delivery_id TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

INSERT INTO pending_ci_events_v2 (
    id, request_id, kind, trigger_kind, event_name, event_key,
    delivery_id, state, summary, created_at
)
SELECT
    id, request_id, kind, trigger_kind, event_name, event_key,
    delivery_id, state, summary, created_at
FROM pending_ci_events_old;

DROP TABLE pending_ci_events_old;
DROP TABLE pending_ci_requests;
ALTER TABLE pending_ci_requests_v2 RENAME TO pending_ci_requests;
ALTER TABLE pending_ci_events_v2 RENAME TO pending_ci_events;

CREATE UNIQUE INDEX pending_ci_one_armed_pr_idx
ON pending_ci_requests (repository_id, pull_request)
WHERE lifecycle = 'armed';

CREATE INDEX pending_ci_due_idx
ON pending_ci_requests (schedule, next_check_at, lease_expires_at, id)
WHERE lifecycle = 'armed';

CREATE INDEX pending_ci_source_idx
ON pending_ci_requests (repository_id, pull_request, source_comment_id, lifecycle);

CREATE INDEX pending_ci_cleanup_due_idx
ON pending_ci_requests (next_check_at, lease_expires_at, id)
WHERE cleanup_pending = 1;

CREATE INDEX pending_ci_events_request_idx
ON pending_ci_events (request_id, id DESC);
