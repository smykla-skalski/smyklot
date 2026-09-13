-- Rebuild only this leaf table to extend SQLite's observation constraint.
CREATE TABLE sync_repository_state_inputs (
 repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
 applied_digest TEXT NOT NULL,
 applied_at TEXT NOT NULL,
 problem TEXT NOT NULL DEFAULT '',
 observation TEXT NOT NULL DEFAULT ''
  CHECK (observation IN ('', 'matched', 'applied', 'proposed', 'declined', 'different', 'failed', 'blocked')),
 observed_digest TEXT NOT NULL DEFAULT '',
 PRIMARY KEY (repository_id, kind)
);
INSERT INTO sync_repository_state_inputs
 (repository_id, kind, applied_digest, applied_at, problem, observation, observed_digest)
SELECT repository_id, kind, applied_digest, applied_at, problem, observation,
 CASE WHEN observation <> '' THEN applied_digest ELSE '' END FROM sync_repository_state;
DROP TABLE sync_repository_state;
ALTER TABLE sync_repository_state_inputs RENAME TO sync_repository_state;
