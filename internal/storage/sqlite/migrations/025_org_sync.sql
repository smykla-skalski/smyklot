-- Organization-wide sync: what is configured, what a plan would do, and what
-- each repository has already had applied.
--
-- Its own tables rather than more columns on repositories.config_patch. That
-- column is thirteen scalar settings resolved one field at a time, and the
-- panel counts its keys to show how many settings a repository overrides; a
-- whole label catalogue in there would read as one override. It also carries a
-- single revision, so saving a label colour would conflict with saving
-- command_prefix, on a different page, edited by somebody else.

CREATE TABLE sync_configs (
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    enabled INTEGER NOT NULL DEFAULT 0,
    document TEXT NOT NULL DEFAULT '{}',
    -- digest fingerprints enabled and document together. Invalidation is then
    -- an equality check rather than a diff, and it is what the browser sends
    -- back when approving so that what was reviewed is what runs.
    digest TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    updated_by TEXT NOT NULL REFERENCES accounts(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (target_id, kind)
);

-- A repository's own answer to one kind, where NULL means it has not given one
-- and inherits. The same nullable-boolean shape repositories.enabled_override
-- already uses, deliberately: one idea, one spelling.
CREATE TABLE sync_repository_overrides (
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    enabled_override INTEGER,
    revision INTEGER NOT NULL DEFAULT 1,
    updated_by TEXT NOT NULL REFERENCES accounts(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (repository_id, kind)
);

CREATE TABLE sync_plans (
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
        state IN ('computed', 'approved', 'applying', 'applied', 'failed', 'stale', 'expired')
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

-- One live plan per installation, as a fact the database holds rather than a
-- convention the callers keep. The reconcile loop cannot then race the panel,
-- and pressing "sync now" twice is naturally idempotent.
CREATE UNIQUE INDEX sync_plans_live_idx
    ON sync_plans (target_id)
    WHERE state IN ('computed', 'approved', 'applying');

CREATE INDEX sync_plans_target_idx ON sync_plans (target_id, computed_at DESC);
CREATE INDEX sync_plans_lease_idx ON sync_plans (state, lease_expires_at);

CREATE TABLE sync_plan_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id TEXT NOT NULL REFERENCES sync_plans(id) ON DELETE CASCADE,
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    operation TEXT NOT NULL CHECK (operation IN ('create', 'update', 'delete')),
    subject TEXT NOT NULL,
    -- What the subject looks like on either side, rendered for a person to
    -- read. Empty before a creation and after a deletion.
    before_state TEXT NOT NULL DEFAULT '',
    after_state TEXT NOT NULL DEFAULT '',
    -- What to apply, as the kind that owns it spells it.
    --
    -- Carried on the action rather than re-read from the configuration when
    -- the work runs, because the plan is the contract between what somebody
    -- reviewed and what happens. Reading the configuration again at apply time
    -- would apply what it says then, which is precisely what the plan exists
    -- to stop.
    payload TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('pending', 'applied', 'failed', 'skipped')),
    error TEXT NOT NULL DEFAULT '',
    -- blocker names the earlier kind whose failure meant this was never tried.
    -- A repository that fails on its labels does not go on to have its files
    -- rewritten, and the plan says which one stopped it.
    blocker TEXT NOT NULL DEFAULT '',
    UNIQUE (plan_id, repository_id, kind, subject)
);

CREATE INDEX sync_plan_actions_plan_idx
    ON sync_plan_actions (plan_id, repository_id, kind, id);

-- What each repository has already had applied, so a planner emits nothing
-- where the digests match. This is what keeps a steady-state reconcile at zero
-- GitHub calls rather than one per repository per kind.
CREATE TABLE sync_repository_state (
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    applied_digest TEXT NOT NULL,
    applied_at TEXT NOT NULL,
    PRIMARY KEY (repository_id, kind)
);
