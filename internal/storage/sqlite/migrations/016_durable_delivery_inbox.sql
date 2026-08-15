ALTER TABLE deliveries ADD COLUMN payload BLOB;
ALTER TABLE deliveries ADD COLUMN next_attempt_at TEXT;
ALTER TABLE deliveries ADD COLUMN lease_expires_at TEXT;
ALTER TABLE deliveries ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX deliveries_ready_idx
ON deliveries (status, next_attempt_at, lease_expires_at, id)
WHERE status = 'running' AND payload IS NOT NULL;
