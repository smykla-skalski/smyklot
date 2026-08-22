-- v1.45.0 shipped the first form of migration 036, whose sync_plans table
-- rebuild cascaded through sync_plan_actions. A database that already recorded
-- version 36 cannot replay the corrected migration, and the missing action
-- payloads cannot be reconstructed from the plan summary.
--
-- Do not leave such a plan in the live slot: its counts promise work that its
-- action list can no longer perform. Removing only mismatched live plans frees
-- the slot for a fresh computation while preserving complete plans and history.
DELETE FROM sync_plans
WHERE state IN ('computed', 'approved', 'applying')
  AND create_count + update_count + delete_count != (
      SELECT COUNT(*)
      FROM sync_plan_actions
      WHERE sync_plan_actions.plan_id = sync_plans.id
  );
