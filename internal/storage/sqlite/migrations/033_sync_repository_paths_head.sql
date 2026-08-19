-- The commit the path list was read at.
--
-- What makes a rescan affordable. The list itself is 2.6 MB for a repository
-- holding eight thousand files, and reading it takes a second; the commit the
-- default branch points at is 342 bytes and answers the only question a
-- refresh has - whether anything has changed since the last one. So a tick
-- costs one small read per repository, and a tree is read only where the
-- branch has actually moved.
--
-- Empty for a row written before this column existed, which reads as "unknown"
-- and rescans once. Empty is not a commit, so nothing has to remember that it
-- means anything else.
ALTER TABLE sync_repository_paths ADD COLUMN head_sha TEXT NOT NULL DEFAULT '';

-- Whether GitHub listed the tree whole.
--
-- Nothing drops a path on purpose - the five-thousand cap that used to sit here
-- threw away 84% of a repository the size of kubernetes and said nothing, and
-- it is gone. This is the one limit that is not ours: GitHub declines to list a
-- very large tree in one answer. Recorded so the panel can say the list is
-- partial, rather than showing a short one that looks complete.
ALTER TABLE sync_repository_paths ADD COLUMN partial INTEGER NOT NULL DEFAULT 0;
