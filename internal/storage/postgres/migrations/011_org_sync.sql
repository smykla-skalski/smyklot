-- Organization-wide sync. The SQLite twin is 025_org_sync.sql, and
-- TestSchemaParity fails if the two drift.

CREATE TABLE sync_configs (
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    -- TEXT rather than JSONB, unlike config_patch beside it.
    --
    -- JSONB stores a parsed document and re-renders it on the way out, so
    -- {"labels":[]} comes back as {"labels": []}. The digest beside this column
    -- is taken from the bytes somebody saved, and a copy between engines moves
    -- the two columns independently - so a document normalised on arrival and a
    -- digest carried across verbatim would disagree the moment they landed.
    --
    -- Nothing queries into this. It is read whole, decoded by the kind that
    -- owns it, and an index over it later can cast on the way in.
    document TEXT NOT NULL DEFAULT '{}',
    digest TEXT NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1,
    updated_by TEXT NOT NULL REFERENCES accounts(id),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (target_id, kind)
);

CREATE TABLE sync_repository_overrides (
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    enabled_override BOOLEAN,
    revision BIGINT NOT NULL DEFAULT 1,
    updated_by TEXT NOT NULL REFERENCES accounts(id),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (repository_id, kind)
);

CREATE TABLE sync_plans (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    trigger_kind TEXT NOT NULL
        CHECK (trigger_kind IN ('manual', 'save', 'reconcile', 'webhook')),
    actor_account_id TEXT NOT NULL REFERENCES accounts(id),
    digest TEXT NOT NULL,
    state TEXT NOT NULL CHECK (
        state IN ('computed', 'approved', 'applying', 'applied', 'failed', 'stale', 'expired')
    ),
    create_count INTEGER NOT NULL DEFAULT 0,
    update_count INTEGER NOT NULL DEFAULT 0,
    delete_count INTEGER NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    lease_expires_at TIMESTAMPTZ,
    attempt INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX sync_plans_live_idx
    ON sync_plans (target_id)
    WHERE state IN ('computed', 'approved', 'applying');

CREATE INDEX sync_plans_target_idx ON sync_plans (target_id, computed_at DESC);
CREATE INDEX sync_plans_lease_idx ON sync_plans (state, lease_expires_at);

CREATE TABLE sync_plan_actions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    plan_id TEXT NOT NULL REFERENCES sync_plans(id) ON DELETE CASCADE,
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    operation TEXT NOT NULL CHECK (operation IN ('create', 'update', 'delete')),
    subject TEXT NOT NULL,
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
    blocker TEXT NOT NULL DEFAULT '',
    UNIQUE (plan_id, repository_id, kind, subject)
);

CREATE INDEX sync_plan_actions_plan_idx
    ON sync_plan_actions (plan_id, repository_id, kind, id);

CREATE TABLE sync_repository_state (
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('labels', 'settings', 'rulesets', 'files')),
    applied_digest TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (repository_id, kind)
);
