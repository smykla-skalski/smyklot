ALTER TABLE runtime_settings ADD COLUMN pending_ci_quiet_period_seconds BIGINT CHECK (
    pending_ci_quiet_period_seconds IS NULL OR pending_ci_quiet_period_seconds >= 1
);
