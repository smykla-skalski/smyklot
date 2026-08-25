ALTER TABLE runtime_settings
ADD COLUMN background_work_paused BOOLEAN NOT NULL DEFAULT FALSE;
