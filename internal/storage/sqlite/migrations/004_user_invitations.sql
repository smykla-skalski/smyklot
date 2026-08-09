CREATE TABLE user_invitations (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    target_id TEXT REFERENCES targets(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('viewer', 'editor', 'admin', 'owner')),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined', 'revoked')),
    expires_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES accounts(id),
    created_at TEXT NOT NULL,
    responded_at TEXT
);

CREATE INDEX user_invitations_scope_idx
    ON user_invitations (target_id, status, created_at DESC);
CREATE INDEX user_invitations_account_idx
    ON user_invitations (account_id, status, created_at DESC);
