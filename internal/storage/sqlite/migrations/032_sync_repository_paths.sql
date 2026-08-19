-- Every path a repository is known to hold, so the panel can offer them.
--
-- Typing a path into an empty box is guessing: somebody is being asked for a
-- string that has to match, character for character, something they cannot see.
-- The finder answers with what exists instead, and this is where "what exists"
-- is kept.
--
-- One row per repository rather than one per path. A path is only interesting
-- beside the others in its repository, the whole list is replaced together
-- whenever it is read again, and fifty thousand rows written per sweep to
-- answer a question nobody asks between sweeps is a cost with no reader.
--
-- The list was newline-joined here, on the reasoning that it is read as a whole
-- and a path cannot contain a newline. The second half of that is wrong: git
-- permits a newline in a filename, and one such file came back as two paths
-- that do not exist. Migration 035 empties the table and it holds JSON from
-- there on. Left as it was written, because a migration is what ran.
CREATE TABLE sync_repository_paths (
    repository_id TEXT PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    paths TEXT NOT NULL,
    observed_at TEXT NOT NULL
);

CREATE INDEX sync_repository_paths_target_idx
ON sync_repository_paths (target_id, repository_id);
