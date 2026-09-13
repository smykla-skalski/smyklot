-- Historical evidence belongs to the check, not the current repository state.
CREATE TABLE sync_check_observations (
    queue_id TEXT NOT NULL REFERENCES queue_items(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    evidence TEXT NOT NULL,
    PRIMARY KEY (queue_id, ordinal)
);
