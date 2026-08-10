CREATE TABLE user_preferences (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    doc        TEXT NOT NULL DEFAULT '{}',
    revision   INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL
);
