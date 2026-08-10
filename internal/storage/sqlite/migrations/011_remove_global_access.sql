INSERT INTO access_audit_entries (
    target_id, actor_account_id, subject_account_id, action, summary, created_at
)
SELECT
    NULL,
    account_id,
    account_id,
    'global_access.migration_revoked',
    'revoked legacy global ' || global_role || ' grant during installation-role migration',
    updated_at
FROM panel_users
WHERE system_role = 'none'
  AND global_role IN ('viewer', 'editor', 'admin');

INSERT INTO app_audit_events (
    category, source_kind, source_id, actor_account_id, subject_account_id,
    action, summary, created_at
)
SELECT
    'access', 'access', id, actor_account_id, subject_account_id,
    action, summary, created_at
FROM access_audit_entries
WHERE action = 'global_access.migration_revoked'
  AND NOT EXISTS (
      SELECT 1 FROM app_audit_events
      WHERE source_kind = 'access' AND source_id = access_audit_entries.id
  );

PRAGMA defer_foreign_keys = ON;

CREATE TABLE target_roles_global_access_backup AS
SELECT
    account_id, target_id, role, suspended, suspension_reason,
    revision, updated_by, updated_at
FROM target_roles;

CREATE TABLE panel_users_without_global_access (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'banned', 'removed')),
    system_role TEXT NOT NULL DEFAULT 'none'
        CHECK (system_role IN ('none', 'root', 'super_root')),
    ban_reason TEXT,
    banned_at TEXT,
    removed_at TEXT,
    last_login_at TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO panel_users_without_global_access (
    account_id, status, system_role, ban_reason, banned_at, removed_at,
    last_login_at, revision, created_at, updated_at
)
SELECT
    account_id, status, system_role, ban_reason, banned_at, removed_at,
    last_login_at, revision, created_at, updated_at
FROM panel_users;

DROP TABLE panel_users;
ALTER TABLE panel_users_without_global_access RENAME TO panel_users;

INSERT INTO target_roles (
    account_id, target_id, role, suspended, suspension_reason,
    revision, updated_by, updated_at
)
SELECT
    account_id, target_id, role, suspended, suspension_reason,
    revision, updated_by, updated_at
FROM target_roles_global_access_backup;

DROP TABLE target_roles_global_access_backup;

CREATE UNIQUE INDEX panel_users_single_super_root_idx
    ON panel_users (system_role)
    WHERE system_role = 'super_root';

CREATE TABLE user_invitations_without_global_access (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    target_id TEXT REFERENCES targets(id) ON DELETE CASCADE,
    role TEXT CHECK (role IS NULL OR role IN ('viewer', 'editor', 'admin', 'owner')),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined', 'revoked')),
    expires_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES accounts(id),
    created_at TEXT NOT NULL,
    responded_at TEXT,
    system_role TEXT CHECK (system_role IN ('root')),
    CHECK (
        (target_id IS NOT NULL AND system_role IS NULL
            AND role IN ('viewer', 'editor', 'admin'))
        OR (target_id IS NULL AND system_role = 'root' AND role IS NULL)
        OR (target_id IS NULL AND system_role IS NULL AND status <> 'pending'
            AND role IN ('viewer', 'editor', 'admin', 'owner'))
    )
);

INSERT INTO user_invitations_without_global_access (
    id, token_hash, account_id, target_id, role, status, expires_at,
    created_by, created_at, responded_at, system_role
)
SELECT
    id,
    token_hash,
    account_id,
    target_id,
    CASE WHEN system_role IS NULL THEN role ELSE NULL END,
    status,
    expires_at,
    created_by,
    created_at,
    responded_at,
    system_role
FROM user_invitations;

DROP TABLE user_invitations;
ALTER TABLE user_invitations_without_global_access RENAME TO user_invitations;

CREATE INDEX user_invitations_scope_idx
    ON user_invitations (target_id, status, created_at DESC);
CREATE INDEX user_invitations_account_idx
    ON user_invitations (account_id, status, created_at DESC);
