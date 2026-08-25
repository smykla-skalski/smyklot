ALTER TABLE runtime_settings
ADD COLUMN background_work_paused INTEGER NOT NULL DEFAULT 0
CHECK (background_work_paused IN (0, 1));
