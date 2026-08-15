CREATE TABLE pending_ci_intents (
    repository_id TEXT NOT NULL,
    pull_request INTEGER NOT NULL,
    source_comment_id BIGINT NOT NULL,
    source_revision TEXT NOT NULL,
    source_sequence INTEGER NOT NULL,
    source_order BIGINT NOT NULL,
    intent_kind TEXT NOT NULL CHECK (intent_kind IN ('arm', 'cancel')),
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (repository_id, pull_request)
);
