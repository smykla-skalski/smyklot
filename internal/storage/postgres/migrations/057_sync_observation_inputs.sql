-- Cache misses also need an input identity: old failures must not block new settings.
ALTER TABLE sync_repository_state ADD COLUMN observed_digest TEXT NOT NULL DEFAULT '';
UPDATE sync_repository_state SET observed_digest = applied_digest WHERE observation <> '';
ALTER TABLE sync_repository_state DROP CONSTRAINT sync_repository_state_observation_check;
ALTER TABLE sync_repository_state ADD CONSTRAINT sync_repository_state_observation_check
 CHECK (observation IN ('', 'matched', 'applied', 'proposed', 'declined', 'different', 'failed', 'blocked'));
