DROP INDEX sync_plans_target_idx;
CREATE INDEX sync_plans_target_idx ON sync_plans (target_id, computed_at DESC, id DESC);
