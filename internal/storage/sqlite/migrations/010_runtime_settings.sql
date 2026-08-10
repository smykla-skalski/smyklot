CREATE TABLE runtime_settings (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    bot_config TEXT,
    poll_interval_seconds INTEGER CHECK (
        poll_interval_seconds IS NULL OR poll_interval_seconds >= 0
    ),
    log_level TEXT CHECK (
        log_level IS NULL OR log_level IN ('debug', 'info', 'warn', 'error')
    ),
    session_ttl_seconds INTEGER CHECK (
        session_ttl_seconds IS NULL OR session_ttl_seconds >= 60
    ),
    revision INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by_account_id TEXT NOT NULL REFERENCES accounts(id)
);
