-- How often a repository's file list is checked for changes.
--
-- Three levels, because the right answer is not the same everywhere: an
-- installation whose repositories are edited all day wants a fresher finder
-- than one holding archived services, and inside either there is usually one
-- repository that is the exception.
--
-- It is a choice at all because of what a check costs. The list is the
-- expensive read - 2.65 MB and 1.2s for a repository holding 8,229 files, 10.3
-- MB for 31,300 - and it is read only where the branch has moved since the last
-- scan. What a tick actually costs is the commit that branch points at, which
-- is 342 bytes whatever the repository's size.
--
-- Zero is every sweep rather than never: an interval of nothing is the plain
-- reading of the word, and nothing here is expensive enough to need an off
-- switch. Seven days is the ceiling, which is what "hardly ever" means for a
-- list somebody types against.
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
