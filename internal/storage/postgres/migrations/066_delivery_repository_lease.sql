CREATE INDEX deliveries_repository_lease_idx
ON deliveries (repository_id, lease_expires_at)
WHERE status = 'running';

CREATE INDEX deliveries_payload_expiry_idx
ON deliveries (finished_at, id)
WHERE status = 'succeeded' AND payload IS NOT NULL;
