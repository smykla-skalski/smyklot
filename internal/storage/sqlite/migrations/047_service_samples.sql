CREATE TABLE service_samples (
    metric TEXT NOT NULL CHECK (metric IN ('query', 'ledger', 'lane', 'database')),
    label TEXT NOT NULL,
    sampled_at TEXT NOT NULL,
    observations INTEGER NOT NULL DEFAULT 0 CHECK (observations >= 0),
    failures INTEGER NOT NULL DEFAULT 0 CHECK (failures >= 0),
    total_millis REAL NOT NULL DEFAULT 0 CHECK (total_millis >= 0),
    max_millis REAL NOT NULL DEFAULT 0 CHECK (max_millis >= 0),
    value REAL NOT NULL DEFAULT 0,
    PRIMARY KEY (metric, label, sampled_at)
);

CREATE INDEX service_samples_window_idx
ON service_samples (metric, sampled_at DESC, label);
