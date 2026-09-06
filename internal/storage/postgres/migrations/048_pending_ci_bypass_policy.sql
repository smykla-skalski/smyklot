ALTER TABLE targets ADD COLUMN pending_ci_bypass_policy_default TEXT;
ALTER TABLE repositories ADD COLUMN pending_ci_bypass_policy_override TEXT;
