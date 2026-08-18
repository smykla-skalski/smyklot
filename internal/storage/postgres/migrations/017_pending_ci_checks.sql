-- Check mode accepts zero quiet time but still requires two observations.
ALTER TABLE runtime_settings
DROP CONSTRAINT IF EXISTS runtime_settings_pending_ci_quiet_period_seconds_check;

ALTER TABLE runtime_settings
ADD CONSTRAINT runtime_settings_pending_ci_quiet_period_seconds_check CHECK (
    pending_ci_quiet_period_seconds IS NULL OR pending_ci_quiet_period_seconds >= 0
);

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

-- One row serializes runtime, target, and repository quiet-policy changes so
-- each transaction derives deadlines from a coherent inheritance chain.
CREATE TABLE pending_ci_policy_lock (
    singleton SMALLINT PRIMARY KEY CHECK (singleton = 1)
);

INSERT INTO pending_ci_policy_lock (singleton) VALUES (1);

CREATE TABLE pending_ci_repository_gates (
    repository_id TEXT PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    desired_mode TEXT NOT NULL CHECK (desired_mode IN ('labels', 'checks')),
    effective_mode TEXT NOT NULL DEFAULT 'none'
        CHECK (effective_mode IN ('none', 'labels', 'checks')),
    readiness TEXT NOT NULL DEFAULT 'provisioning'
        CHECK (readiness IN ('ready', 'provisioning', 'draining', 'blocked')),
    reason TEXT NOT NULL DEFAULT '',
    app_id BIGINT,
    ruleset_id BIGINT,
    ruleset_fingerprint TEXT NOT NULL DEFAULT '',
    generation BIGINT NOT NULL DEFAULT 1,
    observed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1
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

CREATE TABLE pending_ci_check_slots (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    installation_id BIGINT NOT NULL,
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    repository_full_name TEXT NOT NULL,
    pull_request INTEGER NOT NULL,
    head_sha TEXT NOT NULL,
    app_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    external_id TEXT NOT NULL,
    generation BIGINT NOT NULL DEFAULT 1,
    check_run_id BIGINT,
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
    retry_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1,
    UNIQUE (repository_id, head_sha),
    UNIQUE (external_id)
);

CREATE UNIQUE INDEX pending_ci_check_slots_run_idx
ON pending_ci_check_slots (check_run_id)
WHERE check_run_id IS NOT NULL;

CREATE INDEX pending_ci_check_slots_retry_idx
ON pending_ci_check_slots (state, retry_at, id);

ALTER TABLE pending_ci_requests
ADD COLUMN artifact_kind TEXT NOT NULL DEFAULT 'label'
CHECK (artifact_kind IN ('label', 'check'));

ALTER TABLE pending_ci_requests ALTER COLUMN label DROP NOT NULL;

ALTER TABLE pending_ci_requests
ADD COLUMN check_slot_id BIGINT REFERENCES pending_ci_check_slots(id) ON DELETE SET NULL;

ALTER TABLE pending_ci_requests
ADD COLUMN retired_check_slot_id BIGINT REFERENCES pending_ci_check_slots(id) ON DELETE SET NULL;

ALTER TABLE pending_ci_requests
ADD COLUMN authorization_state TEXT NOT NULL DEFAULT 'authorized'
CHECK (authorization_state IN ('authorized', 'reauthorization_required'));

ALTER TABLE pending_ci_requests
ADD COLUMN gate_state TEXT NOT NULL DEFAULT 'ready'
CHECK (gate_state IN ('ready', 'readiness_blocked'));

ALTER TABLE pending_ci_requests ADD COLUMN candidate_head_sha TEXT;
ALTER TABLE pending_ci_requests ADD COLUMN candidate_base_branch TEXT;
ALTER TABLE pending_ci_requests ADD COLUMN authorized_by TEXT;
ALTER TABLE pending_ci_requests ADD COLUMN authorized_at TIMESTAMPTZ;

UPDATE pending_ci_requests
SET authorized_by = requester, authorized_at = requested_at;

ALTER TABLE pending_ci_requests ALTER COLUMN authorized_by SET NOT NULL;
ALTER TABLE pending_ci_requests ALTER COLUMN authorized_at SET NOT NULL;

ALTER TABLE pending_ci_requests
ADD COLUMN merge_phase TEXT NOT NULL DEFAULT 'waiting'
CHECK (merge_phase IN ('waiting', 'claimed', 'check_succeeded'));

ALTER TABLE pending_ci_requests
ADD CONSTRAINT pending_ci_artifact_shape CHECK (
    (artifact_kind = 'label' AND label IS NOT NULL AND check_slot_id IS NULL) OR
    (artifact_kind = 'check' AND label IS NULL AND check_slot_id IS NOT NULL)
);
