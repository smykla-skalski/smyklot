-- Org sync gets its own audit category and its own detail table. The SQLite
-- twin is 026_sync_audit.sql.
--
-- Cheaper here than there, but not free: dropping and re-adding the constraint
-- would scan the whole table under an exclusive lock. Added NOT VALID first and
-- validated after, so the lock is held for an instant and the scan runs beside
-- ordinary traffic.
--
-- The constraint name is looked up rather than guessed. PostgreSQL generates it
-- and guessing is how this fails at three in the morning against a database
-- whose constraint was named by an older release.
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'app_audit_events'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%category%';

    IF constraint_name IS NOT NULL THEN
        EXECUTE format(
            'ALTER TABLE app_audit_events DROP CONSTRAINT %I', constraint_name);
    END IF;
END
$$;

ALTER TABLE app_audit_events
    ADD CONSTRAINT app_audit_events_category_check
    CHECK (
        category IN (
            'configuration', 'access', 'ownership', 'elevation', 'notification',
            'runtime', 'sync'
        )
    ) NOT VALID;

ALTER TABLE app_audit_events VALIDATE CONSTRAINT app_audit_events_category_check;

CREATE TABLE sync_audit_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
    plan_id TEXT NOT NULL,
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    create_count INTEGER NOT NULL DEFAULT 0,
    update_count INTEGER NOT NULL DEFAULT 0,
    delete_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX sync_audit_entries_target_idx ON sync_audit_entries (target_id, id DESC);
CREATE INDEX sync_audit_entries_plan_idx ON sync_audit_entries (plan_id, id);
