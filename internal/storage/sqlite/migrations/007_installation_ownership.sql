CREATE TABLE target_ownership (
    target_id TEXT PRIMARY KEY REFERENCES targets(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('personal', 'organization_admin')),
    status TEXT NOT NULL CHECK (status IN ('fresh', 'permission_pending', 'error')),
    detail TEXT,
    synced_at TEXT NOT NULL
);

CREATE TABLE target_owners (
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    synced_at TEXT NOT NULL,
    PRIMARY KEY (target_id, account_id)
);

CREATE INDEX target_owners_account_idx ON target_owners (account_id, target_id);
