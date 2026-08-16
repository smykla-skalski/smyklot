-- What an installation has granted the App. The SQLite twin is
-- 028_target_permissions.sql.
--
-- TEXT rather than JSONB for the same reason the sync document is: nothing
-- queries into it, it is read whole, and a column that re-renders its contents
-- makes the two engines hand back different bytes for the same row.
ALTER TABLE targets ADD COLUMN permissions TEXT NOT NULL DEFAULT '{}';
