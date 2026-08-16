-- Which file a repository's configuration was read from, and which others were
-- passed over. Discovery looks in four places plus a panel-chosen one, so
-- "valid" no longer says which file it is talking about - and a repository that
-- migrated to TOML and left the YAML behind has a file it believes is in charge
-- and is not.
ALTER TABLE repositories ADD COLUMN config_file_path TEXT NOT NULL DEFAULT '';
ALTER TABLE repositories ADD COLUMN config_file_superseded JSONB NOT NULL DEFAULT '[]';
