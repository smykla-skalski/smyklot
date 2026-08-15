ALTER TABLE pending_ci_requests
ADD COLUMN source_sequence INTEGER NOT NULL DEFAULT 1;

ALTER TABLE pending_ci_requests
ADD COLUMN source_order INTEGER NOT NULL DEFAULT 1;

CREATE TABLE pending_ci_source_revisions (
    repository_id TEXT NOT NULL,
    pull_request INTEGER NOT NULL,
    source_comment_id INTEGER NOT NULL,
    source_revision TEXT NOT NULL,
    source_sequence INTEGER NOT NULL,
    event_key TEXT NOT NULL,
    source_order INTEGER NOT NULL,
    observed_at TEXT NOT NULL,
    PRIMARY KEY (repository_id, pull_request, source_comment_id, event_key),
    UNIQUE (repository_id, pull_request, source_order)
);

CREATE INDEX pending_ci_source_revisions_latest_idx
ON pending_ci_source_revisions (repository_id, pull_request, source_comment_id, source_order DESC);
