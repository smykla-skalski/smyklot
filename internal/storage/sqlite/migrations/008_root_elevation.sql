CREATE TABLE root_elevations (
    id TEXT PRIMARY KEY,
    session_token_hash TEXT NOT NULL,
    root_account_id TEXT NOT NULL REFERENCES accounts(id),
    target_id TEXT NOT NULL REFERENCES targets(id),
    reason TEXT,
    started_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    ended_at TEXT,
    end_reason TEXT CHECK (end_reason IN ('ended', 'expired', 'revoked'))
);

CREATE INDEX root_elevations_session_idx
    ON root_elevations (session_token_hash, ended_at, expires_at);
CREATE INDEX root_elevations_target_idx
    ON root_elevations (target_id, started_at DESC);

CREATE TABLE app_audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL CHECK (
        category IN ('configuration', 'access', 'ownership', 'elevation', 'notification', 'runtime')
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

CREATE INDEX app_audit_events_created_idx ON app_audit_events (id DESC);
CREATE INDEX app_audit_events_target_idx ON app_audit_events (target_id, id DESC);
CREATE INDEX app_audit_events_elevation_idx ON app_audit_events (elevation_id, id);

INSERT INTO app_audit_events (
    category, source_kind, source_id, target_id, actor_account_id,
    action, summary, created_at
)
SELECT
    'configuration', 'settings', id, target_id, actor_account_id,
    action, summary, created_at
FROM audit_entries;

INSERT INTO app_audit_events (
    category, source_kind, source_id, target_id, actor_account_id,
    subject_account_id, action, summary, created_at
)
SELECT
    'access', 'access', id, target_id, actor_account_id,
    subject_account_id, action, summary, created_at
FROM access_audit_entries;

CREATE TABLE security_notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recipient_account_id TEXT NOT NULL REFERENCES accounts(id),
    target_id TEXT NOT NULL REFERENCES targets(id),
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    elevation_id TEXT NOT NULL REFERENCES root_elevations(id),
    audit_event_id INTEGER NOT NULL REFERENCES app_audit_events(id),
    action TEXT NOT NULL,
    reason TEXT,
    created_at TEXT NOT NULL,
    read_at TEXT,
    UNIQUE (recipient_account_id, audit_event_id)
);

CREATE INDEX security_notifications_recipient_idx
    ON security_notifications (recipient_account_id, id DESC);
CREATE INDEX security_notifications_unread_idx
    ON security_notifications (recipient_account_id, read_at, id DESC);
