CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    login TEXT NOT NULL,
    display_name TEXT NOT NULL,
    avatar_url TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE (provider, subject_id)
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE INDEX sessions_account_created_idx ON sessions (account_id, created_at DESC);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE panel_owner (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    account_id TEXT NOT NULL UNIQUE REFERENCES accounts(id)
);

CREATE TABLE targets (
    id TEXT PRIMARY KEY,
    installation_id TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('Organization', 'User')),
    account_id TEXT NOT NULL REFERENCES accounts(id),
    available INTEGER NOT NULL DEFAULT 1 CHECK (available IN (0, 1)),
    repository_default_enabled INTEGER NOT NULL DEFAULT 0 CHECK (repository_default_enabled IN (0, 1)),
    config_patch TEXT NOT NULL DEFAULT '{}',
    revision INTEGER NOT NULL DEFAULT 1,
    settings_updated_at TEXT NOT NULL,
    synced_at TEXT NOT NULL
);

CREATE TABLE target_access (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    verified_at TEXT NOT NULL,
    PRIMARY KEY (account_id, target_id)
);

CREATE TABLE repositories (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    private INTEGER NOT NULL CHECK (private IN (0, 1)),
    available INTEGER NOT NULL DEFAULT 1 CHECK (available IN (0, 1)),
    enabled_override INTEGER CHECK (enabled_override IN (0, 1)),
    config_patch TEXT NOT NULL DEFAULT '{}',
    ignore_repository_file INTEGER NOT NULL DEFAULT 0 CHECK (ignore_repository_file IN (0, 1)),
    config_file_status TEXT NOT NULL DEFAULT 'missing'
        CHECK (config_file_status IN ('missing', 'valid', 'invalid', 'bypassed')),
    config_file_patch TEXT NOT NULL DEFAULT '{}',
    config_file_error TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    settings_updated_at TEXT NOT NULL,
    file_observed_at TEXT,
    synced_at TEXT NOT NULL
);

CREATE INDEX repositories_target_idx ON repositories (target_id, available, full_name);
CREATE UNIQUE INDEX repositories_available_full_name_idx
    ON repositories (target_id, full_name)
    WHERE available = 1;

CREATE TABLE audit_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL,
    repository_full_name TEXT,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX audit_target_id_idx ON audit_entries (target_id, id DESC);

CREATE TABLE deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    claim_key TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    repository_id TEXT,
    repository_full_name TEXT NOT NULL,
    event TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    stage TEXT,
    reason TEXT,
    retryable INTEGER CHECK (retryable IN (0, 1)),
    claimed_at TEXT NOT NULL,
    finished_at TEXT
);

CREATE UNIQUE INDEX deliveries_retained_claim_idx ON deliveries (claim_key)
WHERE status IN ('running', 'succeeded') OR (status = 'failed' AND retryable = 0);
CREATE INDEX deliveries_target_failure_idx ON deliveries (target_id, status, id DESC);
CREATE INDEX deliveries_finished_idx ON deliveries (finished_at);
