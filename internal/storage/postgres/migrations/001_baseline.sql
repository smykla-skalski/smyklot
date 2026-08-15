-- The schema every released SQLite migration adds up to, stated once.
--
-- Replaying that history here would mean carrying steps that never happened on
-- this engine, including a table rebuild that exists only because SQLite could
-- not drop a column. A fresh database has no such past, so it starts at the
-- end state, and a conformance test asserts the two engines agree on it.
--
-- Types are this engine's own. SQLite stores a boolean as 0 or 1, a timestamp
-- as text, and a config patch as a JSON string, because it has nothing better.
-- Here they are boolean, timestamptz and jsonb.

CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    login TEXT NOT NULL,
    display_name TEXT NOT NULL,
    avatar_url TEXT,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (provider, subject_id)
);

CREATE TABLE panel_owner (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    account_id TEXT NOT NULL UNIQUE REFERENCES accounts(id)
);

CREATE TABLE panel_users (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'banned', 'removed')),
    system_role TEXT NOT NULL DEFAULT 'none'
        CHECK (system_role IN ('none', 'root', 'super_root')),
    ban_reason TEXT,
    banned_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX panel_users_single_super_root_idx
    ON panel_users (system_role)
    WHERE system_role = 'super_root';

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoke_code TEXT,
    revoke_reason TEXT
);

CREATE INDEX sessions_account_created_idx ON sessions (account_id, created_at DESC);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE targets (
    id TEXT PRIMARY KEY,
    installation_id TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('Organization', 'User')),
    account_id TEXT NOT NULL REFERENCES accounts(id),
    available BOOLEAN NOT NULL DEFAULT TRUE,
    repository_default_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    config_patch JSONB NOT NULL DEFAULT '{}',
    revision BIGINT NOT NULL DEFAULT 1,
    settings_updated_at TIMESTAMPTZ NOT NULL,
    synced_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE target_ownership (
    target_id TEXT PRIMARY KEY REFERENCES targets(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('personal', 'organization_admin')),
    status TEXT NOT NULL CHECK (status IN ('fresh', 'permission_pending', 'error')),
    detail TEXT,
    synced_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE target_owners (
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    synced_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (target_id, account_id)
);

CREATE INDEX target_owners_account_idx ON target_owners (account_id, target_id);

CREATE TABLE target_roles (
    account_id TEXT NOT NULL REFERENCES panel_users(account_id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    role TEXT CHECK (role IN ('none', 'viewer', 'editor', 'admin')),
    suspended BOOLEAN NOT NULL DEFAULT FALSE,
    suspension_reason TEXT,
    revision BIGINT NOT NULL DEFAULT 1,
    updated_by TEXT NOT NULL REFERENCES accounts(id),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (account_id, target_id)
);

CREATE INDEX target_roles_target_idx ON target_roles (target_id, account_id);

CREATE TABLE repositories (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    private BOOLEAN NOT NULL,
    available BOOLEAN NOT NULL DEFAULT TRUE,
    enabled_override BOOLEAN,
    config_patch JSONB NOT NULL DEFAULT '{}',
    ignore_repository_file BOOLEAN NOT NULL DEFAULT FALSE,
    config_file_status TEXT NOT NULL DEFAULT 'missing'
        CHECK (config_file_status IN ('missing', 'valid', 'invalid', 'bypassed')),
    config_file_patch JSONB NOT NULL DEFAULT '{}',
    config_file_error TEXT,
    revision BIGINT NOT NULL DEFAULT 1,
    settings_updated_at TIMESTAMPTZ NOT NULL,
    file_observed_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ NOT NULL,
    default_branch TEXT NOT NULL DEFAULT ''
);

CREATE INDEX repositories_target_idx ON repositories (target_id, available, full_name);
CREATE UNIQUE INDEX repositories_available_full_name_idx
    ON repositories (target_id, full_name)
    WHERE available;
CREATE INDEX repositories_target_name_page_idx
    ON repositories (target_id, available, lower(full_name), id);
CREATE INDEX repositories_target_updated_page_idx
    ON repositories (target_id, available, settings_updated_at, id);
-- Repository pages can filter on which config keys a patch overrides, which is
-- a containment question this engine can answer from an index.
CREATE INDEX repositories_config_patch_idx ON repositories USING GIN (config_patch);

CREATE TABLE root_elevations (
    id TEXT PRIMARY KEY,
    session_token_hash TEXT NOT NULL,
    root_account_id TEXT NOT NULL REFERENCES accounts(id),
    target_id TEXT NOT NULL REFERENCES targets(id),
    reason TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    end_reason TEXT CHECK (end_reason IN ('ended', 'expired', 'revoked'))
);

CREATE INDEX root_elevations_session_idx
    ON root_elevations (session_token_hash, ended_at, expires_at);
CREATE INDEX root_elevations_target_idx
    ON root_elevations (target_id, started_at DESC);

CREATE TABLE audit_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL,
    repository_full_name TEXT,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX audit_target_id_idx ON audit_entries (target_id, id DESC);

CREATE TABLE access_audit_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    subject_account_id TEXT NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX access_audit_subject_idx ON access_audit_entries (subject_account_id, id DESC);
CREATE INDEX access_audit_target_idx ON access_audit_entries (target_id, id DESC);

CREATE TABLE app_audit_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    category TEXT NOT NULL CHECK (
        category IN ('configuration', 'access', 'ownership', 'elevation', 'notification', 'runtime')
    ),
    source_kind TEXT,
    source_id BIGINT,
    target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    subject_account_id TEXT REFERENCES accounts(id),
    elevation_id TEXT REFERENCES root_elevations(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (source_kind, source_id)
);

CREATE INDEX app_audit_events_created_idx ON app_audit_events (id DESC);
CREATE INDEX app_audit_events_elevation_idx ON app_audit_events (elevation_id, id);
CREATE INDEX app_audit_events_target_idx ON app_audit_events (target_id, id DESC);

CREATE TABLE security_notifications (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recipient_account_id TEXT NOT NULL REFERENCES accounts(id),
    target_id TEXT NOT NULL REFERENCES targets(id),
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    elevation_id TEXT NOT NULL REFERENCES root_elevations(id),
    audit_event_id BIGINT NOT NULL REFERENCES app_audit_events(id),
    action TEXT NOT NULL,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    read_at TIMESTAMPTZ,
    UNIQUE (recipient_account_id, audit_event_id)
);

CREATE INDEX security_notifications_recipient_idx
    ON security_notifications (recipient_account_id, id DESC);
CREATE INDEX security_notifications_unread_idx
    ON security_notifications (recipient_account_id, read_at, id DESC);

CREATE TABLE deliveries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    claim_key TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    repository_id TEXT,
    repository_full_name TEXT NOT NULL,
    event TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    stage TEXT,
    reason TEXT,
    retryable BOOLEAN,
    claimed_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);

-- The claim is accepted once. This index is what makes the insert's
-- ON CONFLICT DO NOTHING recognize an event that has already been answered,
-- while still allowing a retryable failure to be claimed again.
CREATE UNIQUE INDEX deliveries_retained_claim_idx ON deliveries (claim_key)
    WHERE status IN ('running', 'succeeded') OR (status = 'failed' AND retryable = FALSE);
CREATE INDEX deliveries_target_failure_idx ON deliveries (target_id, status, id DESC);
CREATE INDEX deliveries_finished_idx ON deliveries (finished_at);

CREATE TABLE user_invitations (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    target_id TEXT REFERENCES targets(id) ON DELETE CASCADE,
    role TEXT CHECK (role IS NULL OR role IN ('viewer', 'editor', 'admin', 'owner')),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined', 'revoked')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL REFERENCES accounts(id),
    created_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,
    system_role TEXT CHECK (system_role IN ('root')),
    CHECK (
        (target_id IS NOT NULL AND system_role IS NULL
            AND role IN ('viewer', 'editor', 'admin'))
        OR (target_id IS NULL AND system_role = 'root' AND role IS NULL)
        OR (target_id IS NULL AND system_role IS NULL AND status <> 'pending'
            AND role IN ('viewer', 'editor', 'admin', 'owner'))
    )
);

CREATE INDEX user_invitations_account_idx
    ON user_invitations (account_id, status, created_at DESC);
CREATE INDEX user_invitations_scope_idx
    ON user_invitations (target_id, status, created_at DESC);

CREATE TABLE runtime_settings (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    bot_config TEXT,
    log_level TEXT CHECK (
        log_level IS NULL OR log_level IN ('debug', 'info', 'warn', 'error')
    ),
    session_ttl_seconds BIGINT CHECK (
        session_ttl_seconds IS NULL OR session_ttl_seconds >= 60
    ),
    poll_interval_seconds BIGINT CHECK (
        poll_interval_seconds IS NULL OR poll_interval_seconds >= 0
    ),
    revision BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    updated_by_account_id TEXT NOT NULL REFERENCES accounts(id)
);

CREATE TABLE user_preferences (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    doc JSONB NOT NULL DEFAULT '{}',
    revision BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL
);
