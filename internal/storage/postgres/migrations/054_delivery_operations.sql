-- One original event can have several execution runs. The current run is a
-- transactional cursor; terminal run rows remain unchanged when another starts.
CREATE TABLE delivery_operations (
    claim_key TEXT PRIMARY KEY,
    target_id TEXT NOT NULL,
    source_order BIGINT CHECK (source_order > 0),
    current_delivery_id BIGINT REFERENCES deliveries(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE INDEX delivery_operations_current_idx ON delivery_operations (current_delivery_id);

INSERT INTO delivery_operations (claim_key, target_id, source_order, current_delivery_id)
SELECT d.claim_key, d.target_id,
       (SELECT MIN(COALESCE(prior.source_order, prior.id)) FROM deliveries prior
        WHERE prior.claim_key = d.claim_key AND prior.target_id = d.target_id), d.id
FROM deliveries d
WHERE d.id = (SELECT MAX(latest.id) FROM deliveries latest WHERE latest.claim_key = d.claim_key);

DROP INDEX deliveries_retained_claim_idx;
CREATE UNIQUE INDEX deliveries_retained_claim_idx ON deliveries (claim_key)
WHERE status IN ('running', 'succeeded');
