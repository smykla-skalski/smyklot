CREATE TABLE pending_ci_intents_with_draft (
    repository_id TEXT NOT NULL,
    pull_request INTEGER NOT NULL,
    source_comment_id INTEGER NOT NULL,
    source_revision TEXT NOT NULL,
    source_sequence INTEGER NOT NULL,
    source_order INTEGER NOT NULL,
    intent_kind TEXT NOT NULL CHECK (intent_kind IN ('arm', 'cancel', 'draft')),
    recorded_at TEXT NOT NULL,
    PRIMARY KEY (repository_id, pull_request)
);

INSERT INTO pending_ci_intents_with_draft (
    repository_id, pull_request, source_comment_id, source_revision,
    source_sequence, source_order, intent_kind, recorded_at
)
SELECT repository_id, pull_request, source_comment_id, source_revision,
       source_sequence, source_order, intent_kind, recorded_at
FROM pending_ci_intents;

DROP TABLE pending_ci_intents;

ALTER TABLE pending_ci_intents_with_draft RENAME TO pending_ci_intents;
