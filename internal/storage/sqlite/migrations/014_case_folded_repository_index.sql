-- Repository pages order by lower(full_name) now that the query is written
-- once for every engine, and COLLATE NOCASE is SQLite's alone. An index
-- declared over the old collation no longer serves that order, so it is
-- replaced by one declared over the same expression the query uses.
DROP INDEX IF EXISTS repositories_target_name_page_idx;

CREATE INDEX repositories_target_name_page_idx
    ON repositories (target_id, available, lower(full_name), id);
