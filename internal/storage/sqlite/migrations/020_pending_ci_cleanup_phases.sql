ALTER TABLE pending_ci_requests
ADD COLUMN cleanup_artifacts_done INTEGER NOT NULL DEFAULT 0
CHECK (cleanup_artifacts_done IN (0, 1));
