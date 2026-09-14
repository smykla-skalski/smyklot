-- Completed comparisons have a domain lifetime independent of worker rows.
-- Preserve existing IDs and bytes. Already-pruned evidence cannot be recovered.
CREATE TABLE sync_check_results (
    check_id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL,
    details TEXT NOT NULL
);
CREATE INDEX sync_check_results_target_idx ON sync_check_results (target_id, check_id);
INSERT INTO sync_check_results (check_id, target_id, details)
SELECT id, target_id, details FROM queue_items
WHERE kind = 'sync_scan' AND target_id IS NOT NULL AND (json_type(details, '$.outcome') = 'object' OR json_extract(details, '$.result_plan_id') <> '');

CREATE TABLE sync_check_observations_retained (
    queue_id TEXT NOT NULL REFERENCES sync_check_results(check_id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    evidence TEXT NOT NULL,
    PRIMARY KEY (queue_id, ordinal)
);
INSERT INTO sync_check_observations_retained (queue_id, ordinal, evidence)
SELECT queue_id, ordinal, evidence FROM sync_check_observations;
DROP TABLE sync_check_observations;
ALTER TABLE sync_check_observations_retained RENAME TO sync_check_observations;
