-- A plan somebody read and declined.
--
-- Until now the only ways off the live slot were time (expired), a
-- configuration change underneath it (stale), or applying it. A person who has
-- read a plan and wants none of it had nothing to press, and the panel's
-- Discard button worked only against the development mock. Discarding is an
-- act, so it gets its own word rather than borrowing "expired" - a person
-- reading history should see who declined it, not a timer running out.
--
-- The constraint is found by its definition rather than assumed by name,
-- the same way 012_sync_audit.sql widened the audit trunk's category check:
-- the baseline declares it inline, so the name is whatever the engine minted.
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'sync_plans'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%state%';

    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE sync_plans DROP CONSTRAINT %I', constraint_name);
    END IF;
END
$$;

ALTER TABLE sync_plans
    ADD CONSTRAINT sync_plans_state_check
    CHECK (
        state IN (
            'computed', 'approved', 'applying', 'applied', 'failed', 'stale',
            'expired', 'discarded'
        )
    );
