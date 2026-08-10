ALTER TABLE runtime_settings ADD COLUMN poll_interval_seconds INTEGER CHECK (
    poll_interval_seconds IS NULL OR poll_interval_seconds >= 0
);
