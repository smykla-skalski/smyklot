-- Acceptance discovery is independent of retained queue rows and browser keys.
ALTER TABLE recurring_request_receipts ADD COLUMN kind TEXT NOT NULL DEFAULT '';
ALTER TABLE recurring_request_receipts ADD COLUMN target_id TEXT;
ALTER TABLE recurring_request_receipts ADD COLUMN queue_id TEXT NOT NULL DEFAULT '';
ALTER TABLE recurring_request_receipts ADD COLUMN reason TEXT NOT NULL DEFAULT '';
UPDATE recurring_request_receipts SET kind = (input_json::jsonb ->> 'Kind'), target_id = (input_json::jsonb ->> 'TargetID'), queue_id = (accepted_item::jsonb ->> 'id'), reason = (input_json::jsonb ->> 'Reason');
-- Use the native timestamp type shared by dispatch acceptance.
ALTER TABLE recurring_request_receipts ALTER COLUMN requested_at TYPE TIMESTAMPTZ USING requested_at::timestamptz;
CREATE INDEX recurring_receipt_discovery_idx ON recurring_request_receipts
 (actor_account_id, target_id, kind, requested_at DESC, request_key DESC);
CREATE INDEX dispatch_receipt_discovery_idx ON sync_dispatch_receipts
 (actor_account_id, target_id, accepted_at DESC, request_key DESC);
