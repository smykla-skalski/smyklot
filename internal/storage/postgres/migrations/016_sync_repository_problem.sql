-- Why a repository is not being synced for one kind. The SQLite twin is
-- 030_sync_repository_problem.sql.
ALTER TABLE sync_repository_state ADD COLUMN problem TEXT NOT NULL DEFAULT '';
