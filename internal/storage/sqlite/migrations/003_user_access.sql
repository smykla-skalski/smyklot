CREATE TABLE panel_users (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    root INTEGER NOT NULL DEFAULT 0 CHECK (root IN (0, 1)),
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'banned', 'removed')),
    global_role TEXT NOT NULL DEFAULT 'none'
        CHECK (global_role IN ('none', 'viewer', 'editor', 'admin', 'owner')),
    ban_reason TEXT,
    banned_at TEXT,
    removed_at TEXT,
    last_login_at TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (root = 0 OR global_role = 'owner')
);

INSERT INTO panel_users (
    account_id,
    root,
    status,
    global_role,
    revision,
    created_at,
    updated_at
)
SELECT
    po.account_id,
    1,
    'active',
    'owner',
    1,
    a.updated_at,
    a.updated_at
FROM panel_owner po
JOIN accounts a ON a.id = po.account_id;

CREATE TABLE target_roles (
    account_id TEXT NOT NULL REFERENCES panel_users(account_id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    role TEXT CHECK (role IN ('none', 'viewer', 'editor', 'admin')),
    suspended INTEGER NOT NULL DEFAULT 0 CHECK (suspended IN (0, 1)),
    suspension_reason TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    updated_by TEXT NOT NULL REFERENCES accounts(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (account_id, target_id)
);

CREATE INDEX target_roles_target_idx ON target_roles (target_id, account_id);

CREATE TABLE access_audit_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    subject_account_id TEXT NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX access_audit_target_idx ON access_audit_entries (target_id, id DESC);
CREATE INDEX access_audit_subject_idx ON access_audit_entries (subject_account_id, id DESC);

ALTER TABLE sessions ADD COLUMN revoked_at TEXT;
ALTER TABLE sessions ADD COLUMN revoke_code TEXT;
ALTER TABLE sessions ADD COLUMN revoke_reason TEXT;

DROP TABLE target_access;
