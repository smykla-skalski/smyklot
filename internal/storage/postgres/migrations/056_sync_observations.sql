-- Existing digests conflate matching files, open proposals and closed proposals.
-- Leave them unclassified until the next observation; do not invent evidence.
ALTER TABLE sync_repository_state ADD COLUMN observation TEXT NOT NULL DEFAULT ''
 CHECK (observation IN ('', 'matched', 'applied', 'proposed', 'declined'));
