-- What one repository adjusts about one kind of sync. The SQLite twin is
-- 029_sync_repository_document.sql.
ALTER TABLE sync_repository_overrides ADD COLUMN document TEXT NOT NULL DEFAULT '{}';
