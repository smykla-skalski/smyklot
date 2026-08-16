-- Org sync gets its own audit category and its own detail table.
--
-- Its own category rather than borrowing `configuration`, which today means
-- somebody changed a setting in the panel. The history filter is how an
-- operator finds what happened, and folding an org-wide sweep into the same
-- chip would make both harder to read.
--
-- The cost is that SQLite cannot alter a CHECK, so this rebuilds the largest
-- table in the schema - and that table is a parent: security_notifications
-- points at its ids.
--
-- Deferring the foreign keys is not enough on its own, which a test proved
-- before this shipped. Dropping the parent counts every child row as a
-- violation, and re-creating the rows under a different table name never
-- decrements that count, so the transaction fails at COMMIT however faithfully
-- the ids were copied. PRAGMA foreign_keys cannot be turned off to get around
-- it either: it is a no-op inside a transaction, and the runner wraps every
-- migration in one.
--
-- So the children are moved out of the way first and put back afterwards, which
-- is the same manoeuvre migration 011 uses for target_roles. Nothing references
-- the table while it is being replaced, and the rows land again once their
-- parents are there.

PRAGMA defer_foreign_keys = ON;

CREATE TABLE security_notifications_audit_rebuild AS
SELECT * FROM security_notifications;

DELETE FROM security_notifications;

CREATE TABLE app_audit_events_with_sync (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL CHECK (
        category IN (
            'configuration', 'access', 'ownership', 'elevation', 'notification',
            'runtime', 'sync'
        )
    ),
    source_kind TEXT,
    source_id INTEGER,
    target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    subject_account_id TEXT REFERENCES accounts(id),
    elevation_id TEXT REFERENCES root_elevations(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (source_kind, source_id)
);

-- The ids come across explicitly. security_notifications points at them, and an
-- AUTOINCREMENT column handed fresh ones would leave every notification
-- pointing at the wrong event - or at nothing.
INSERT INTO app_audit_events_with_sync (
    id, category, source_kind, source_id, target_id, actor_account_id,
    subject_account_id, elevation_id, action, summary, created_at
)
SELECT
    id, category, source_kind, source_id, target_id, actor_account_id,
    subject_account_id, elevation_id, action, summary, created_at
FROM app_audit_events;

DROP TABLE app_audit_events;

ALTER TABLE app_audit_events_with_sync RENAME TO app_audit_events;

CREATE INDEX app_audit_events_created_idx ON app_audit_events (id DESC);
CREATE INDEX app_audit_events_target_idx ON app_audit_events (target_id, id DESC);
CREATE INDEX app_audit_events_elevation_idx ON app_audit_events (elevation_id, id);

INSERT INTO security_notifications (
    id, recipient_account_id, target_id, actor_account_id, elevation_id,
    audit_event_id, action, reason, created_at, read_at
)
SELECT
    id, recipient_account_id, target_id, actor_account_id, elevation_id,
    audit_event_id, action, reason, created_at, read_at
FROM security_notifications_audit_rebuild;

DROP TABLE security_notifications_audit_rebuild;

-- What a sync did, in the detail table the trunk points at.
--
-- A plan writes at most three of these: computed, optionally approved, and one
-- terminal outcome, plus one more when anything was deleted - deletion is off
-- by default and should never be silent. A plan that found nothing to do writes
-- none at all, because a reconcile that changed nothing is not an event and one
-- row a tick would be about a hundred and seventy-five thousand a year per
-- installation saying so.
CREATE TABLE sync_audit_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
    plan_id TEXT NOT NULL,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    create_count INTEGER NOT NULL DEFAULT 0,
    update_count INTEGER NOT NULL DEFAULT 0,
    delete_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE INDEX sync_audit_entries_target_idx ON sync_audit_entries (target_id, id DESC);
CREATE INDEX sync_audit_entries_plan_idx ON sync_audit_entries (plan_id, id);
