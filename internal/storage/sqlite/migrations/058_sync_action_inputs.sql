-- Bind execution outcomes to planned inputs, never to settings re-read later.
-- Existing action payloads cannot prove their original input identity.
ALTER TABLE sync_plan_actions ADD COLUMN input_digest TEXT NOT NULL DEFAULT '';
