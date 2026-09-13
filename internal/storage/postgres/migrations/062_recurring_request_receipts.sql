-- Keep acceptance independent of queue retention. Pruning an occurrence must not
-- let a repeated request create new work.
CREATE TABLE recurring_request_receipts (
 actor_account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 request_key TEXT NOT NULL,
 input_json TEXT NOT NULL,
 accepted_item TEXT NOT NULL,
 requested_at TEXT NOT NULL,
 PRIMARY KEY (actor_account_id, request_key)
);
