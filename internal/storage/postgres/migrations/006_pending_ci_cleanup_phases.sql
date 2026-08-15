ALTER TABLE pending_ci_requests
ADD COLUMN cleanup_artifacts_done BOOLEAN NOT NULL DEFAULT FALSE;
