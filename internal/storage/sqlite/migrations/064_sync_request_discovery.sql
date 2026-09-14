-- Acceptance discovery is independent of retained queue rows and browser keys.
ALTER TABLE recurring_request_receipts ADD COLUMN kind TEXT NOT NULL DEFAULT '';
ALTER TABLE recurring_request_receipts ADD COLUMN target_id TEXT;
ALTER TABLE recurring_request_receipts ADD COLUMN queue_id TEXT NOT NULL DEFAULT '';
ALTER TABLE recurring_request_receipts ADD COLUMN reason TEXT NOT NULL DEFAULT '';
UPDATE recurring_request_receipts SET kind = json_extract(input_json, '$.Kind'), target_id = json_extract(input_json, '$.TargetID'), queue_id = json_extract(accepted_item, '$.id'), reason = json_extract(input_json, '$.Reason');
CREATE INDEX recurring_receipt_discovery_idx ON recurring_request_receipts
 (actor_account_id, target_id, kind, requested_at DESC, request_key DESC);
CREATE INDEX dispatch_receipt_discovery_idx ON sync_dispatch_receipts
 (actor_account_id, target_id, accepted_at DESC, request_key DESC);
