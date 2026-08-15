CREATE TABLE pending_ci_requests (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id TEXT NOT NULL,
    installation_id BIGINT NOT NULL,
    repository_id TEXT NOT NULL,
    repository_full_name TEXT NOT NULL,
    pull_request INTEGER NOT NULL,
    head_sha TEXT NOT NULL,
    base_branch TEXT NOT NULL,
    merge_method TEXT NOT NULL CHECK (merge_method IN ('merge', 'squash', 'rebase')),
    required_checks_only BOOLEAN NOT NULL,
    requester TEXT NOT NULL,
    source_comment_id BIGINT NOT NULL,
    source_revision TEXT NOT NULL,
    label TEXT NOT NULL,
    lifecycle TEXT NOT NULL CHECK (lifecycle IN ('armed', 'merged', 'cancelled', 'superseded')),
    schedule TEXT NOT NULL CHECK (schedule IN ('active', 'deferred')),
    next_check_at TIMESTAMPTZ NOT NULL,
    lease_expires_at TIMESTAMPTZ,
    last_progress_at TIMESTAMPTZ NOT NULL,
    last_observed_state TEXT NOT NULL DEFAULT '',
    last_fingerprint TEXT NOT NULL DEFAULT '',
    last_event_key TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    revision BIGINT NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX pending_ci_one_armed_pr_idx
ON pending_ci_requests (repository_id, pull_request)
WHERE lifecycle = 'armed';

CREATE INDEX pending_ci_due_idx
ON pending_ci_requests (schedule, next_check_at, lease_expires_at, id)
WHERE lifecycle = 'armed';

CREATE INDEX pending_ci_source_idx
ON pending_ci_requests (repository_id, pull_request, source_comment_id, lifecycle);
