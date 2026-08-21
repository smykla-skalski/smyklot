-- A plan somebody read and declined.
--
-- Until now the only ways off the live slot were time (expired), a
-- configuration change underneath it (stale), or applying it. A person who has
-- read a plan and wants none of it had nothing to press, and the panel's
-- Discard button worked only against the development mock. Discarding is an
-- act, so it gets its own word rather than borrowing "expired" - a person
-- reading history should see who declined it, not a timer running out.
--
-- SQLite cannot widen a CHECK in place, so the table is rebuilt the way
-- 026_sync_audit.sql rebuilt the audit trunk: a copy with the wider list, the
-- rows carried across, the old table dropped and the copy renamed under it.
CREATE TABLE sync_plans_with_discard (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    -- trigger_kind rather than trigger: the bare word is a keyword both engines
    -- reserve for CREATE TRIGGER, and a column named after it is a quoting
    -- problem waiting for the one query that forgets.
    trigger_kind TEXT NOT NULL
        CHECK (trigger_kind IN ('manual', 'save', 'reconcile', 'webhook')),
    -- Whoever is accountable. For a reconcile that is whoever last saved the
    -- configuration being enforced, carried onto the plan rather than replaced
    -- by a synthetic account: the sweep is doing what they asked for, on a timer.
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    digest TEXT NOT NULL,
    state TEXT NOT NULL CHECK (
        state IN (
            'computed', 'approved', 'applying', 'applied', 'failed', 'stale',
            'expired', 'discarded'
        )
    ),
    create_count INTEGER NOT NULL DEFAULT 0,
    update_count INTEGER NOT NULL DEFAULT 0,
    delete_count INTEGER NOT NULL DEFAULT 0,
    computed_at TEXT NOT NULL,
    approved_at TEXT,
    started_at TEXT,
    finished_at TEXT,
    -- expires_at is time and state='stale' is content. Kept apart because a
    -- person reading "expired" knows to press the button again, and a person
    -- reading "stale" knows somebody changed something.
    expires_at TEXT NOT NULL,
    lease_expires_at TEXT,
    attempt INTEGER NOT NULL DEFAULT 0
);

INSERT INTO sync_plans_with_discard (
    id, target_id, trigger_kind, actor_account_id, digest, state,
    create_count, update_count, delete_count,
    computed_at, approved_at, started_at, finished_at,
    expires_at, lease_expires_at, attempt
)
SELECT
    id, target_id, trigger_kind, actor_account_id, digest, state,
    create_count, update_count, delete_count,
    computed_at, approved_at, started_at, finished_at,
    expires_at, lease_expires_at, attempt
FROM sync_plans;

DROP TABLE sync_plans;

ALTER TABLE sync_plans_with_discard RENAME TO sync_plans;

-- One live plan per installation, as a fact the database holds rather than a
-- convention the callers keep. A discarded plan leaves the slot, which is the
-- point of discarding one.
CREATE UNIQUE INDEX sync_plans_live_idx
    ON sync_plans (target_id)
    WHERE state IN ('computed', 'approved', 'applying');

CREATE INDEX sync_plans_target_idx ON sync_plans (target_id, computed_at DESC);
CREATE INDEX sync_plans_lease_idx ON sync_plans (state, lease_expires_at);
