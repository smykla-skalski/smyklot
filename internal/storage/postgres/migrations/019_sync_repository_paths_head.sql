-- The commit the path list was read at. See the SQLite migration of the same
-- name for why a refresh reads this instead of the tree.
ALTER TABLE sync_repository_paths ADD COLUMN head_sha TEXT NOT NULL DEFAULT '';

-- Whether GitHub listed the tree whole. See the SQLite migration of the same
-- name for why the cap that used to sit beside this is gone.
ALTER TABLE sync_repository_paths ADD COLUMN partial BOOLEAN NOT NULL DEFAULT FALSE;
