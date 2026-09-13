-- Receipts live as long as the original history that permits the request.
-- The successor may be pruned first; its receipt must still prevent replay.
CREATE TABLE delivery_recovery_receipts (
 actor_account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 request_key TEXT NOT NULL,
 source_run_id INTEGER NOT NULL REFERENCES deliveries(id) ON DELETE CASCADE,
 expected_run_id INTEGER NOT NULL,
 expected_revision INTEGER NOT NULL,
 run_id INTEGER NOT NULL,
 requested_at TEXT NOT NULL,
 PRIMARY KEY (actor_account_id, request_key)
);
CREATE INDEX delivery_recovery_source_idx ON delivery_recovery_receipts(source_run_id);
