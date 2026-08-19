-- How often a repository's file list is checked for changes. See the SQLite
-- migration of the same name for why there are three levels and why zero means
-- every sweep.
ALTER TABLE runtime_settings ADD COLUMN path_index_interval_seconds INTEGER CHECK (
    path_index_interval_seconds IS NULL OR
    path_index_interval_seconds BETWEEN 0 AND 604800
);

ALTER TABLE targets
ADD COLUMN path_index_interval_seconds_override INTEGER
CHECK (
    path_index_interval_seconds_override IS NULL OR
    path_index_interval_seconds_override BETWEEN 0 AND 604800
);

ALTER TABLE repositories
ADD COLUMN path_index_interval_seconds_override INTEGER
CHECK (
    path_index_interval_seconds_override IS NULL OR
    path_index_interval_seconds_override BETWEEN 0 AND 604800
);
