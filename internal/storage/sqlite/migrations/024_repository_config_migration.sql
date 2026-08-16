-- Whether Smyklot has proposed moving a repository's configuration to TOML,
-- and what came of it. A pull request somebody closed is a refusal, and asking
-- again every sweep tick would be the bot nagging - so the refusal is durable,
-- keyed on the repository id rather than its name so that a rename does not
-- read as a repository nobody has asked yet.
--
-- 'blocked' is the same durability for a different reason: GitHub refused the
-- push outright, which costs seven requests to discover and would cost them
-- again every tick. Both are cleared from the panel.
ALTER TABLE repositories ADD COLUMN config_migration TEXT NOT NULL DEFAULT 'none'
    CHECK (config_migration IN ('none', 'proposed', 'declined', 'blocked'));
ALTER TABLE repositories ADD COLUMN config_migration_pr INTEGER;
