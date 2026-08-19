-- The stored file list becomes JSON, so a path holding a newline survives it.
-- See the SQLite migration of the same name for why the rows are dropped
-- rather than converted.
DELETE FROM sync_repository_paths;
