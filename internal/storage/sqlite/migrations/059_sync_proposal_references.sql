-- Keep the destination returned by GitHub, including in completed plan history.
-- Older rows remain unknown rather than guessing a branch or pull request.
ALTER TABLE sync_repository_state ADD COLUMN proposal_url TEXT NOT NULL DEFAULT '';
ALTER TABLE sync_plan_actions ADD COLUMN proposal_url TEXT NOT NULL DEFAULT '';
