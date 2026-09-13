-- Initial runs use their own ID. Redeliveries retain that original order even
-- when earlier execution rows expire. NULL represents an initial run.
ALTER TABLE deliveries ADD COLUMN source_order INTEGER CHECK (source_order > 0);

CREATE INDEX deliveries_source_order_idx
ON deliveries (claim_key, target_id, source_order, id);
