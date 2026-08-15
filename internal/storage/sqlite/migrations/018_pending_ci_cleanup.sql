ALTER TABLE pending_ci_requests
ADD COLUMN cleanup_pending INTEGER NOT NULL DEFAULT 0 CHECK (cleanup_pending IN (0, 1));

ALTER TABLE pending_ci_requests
ADD COLUMN cleanup_attempts INTEGER NOT NULL DEFAULT 0;

ALTER TABLE pending_ci_requests
ADD COLUMN cleanup_error TEXT NOT NULL DEFAULT '';

UPDATE pending_ci_requests
SET cleanup_pending = 1,
    next_check_at = COALESCE(finished_at, updated_at)
WHERE lifecycle <> 'armed';

CREATE INDEX pending_ci_cleanup_due_idx
ON pending_ci_requests (next_check_at, lease_expires_at, id)
WHERE cleanup_pending = 1;
