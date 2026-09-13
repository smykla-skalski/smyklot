-- Acceptance outlives the plan and queue records. Current access is checked by
-- the caller before returning it, independently of historical retention.
CREATE TABLE sync_dispatch_receipts (
 actor_account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 request_key TEXT NOT NULL,
 target_id TEXT NOT NULL,
 plan_id TEXT NOT NULL,
 expected_revision BIGINT NOT NULL,
 reason TEXT NOT NULL,
 queue_id TEXT NOT NULL,
 accepted_at TEXT NOT NULL,
 PRIMARY KEY (actor_account_id, request_key)
);
