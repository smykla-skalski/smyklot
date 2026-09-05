CREATE TABLE service_samples (
    metric TEXT NOT NULL CHECK (metric IN ('query', 'ledger', 'lane', 'database')),
    label TEXT NOT NULL,
    sampled_at TIMESTAMPTZ NOT NULL,
    observations BIGINT NOT NULL DEFAULT 0 CHECK (observations >= 0),
    failures BIGINT NOT NULL DEFAULT 0 CHECK (failures >= 0),
    total_nanos BIGINT NOT NULL DEFAULT 0 CHECK (total_nanos >= 0),
    max_nanos BIGINT NOT NULL DEFAULT 0 CHECK (max_nanos >= 0),
    value DOUBLE PRECISION NOT NULL DEFAULT 0,
    PRIMARY KEY (metric, label, sampled_at)
);

CREATE INDEX service_samples_window_idx
ON service_samples (metric, sampled_at, label, total_nanos, max_nanos, value);

CREATE INDEX service_samples_age_idx
ON service_samples (sampled_at);
