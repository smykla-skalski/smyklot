-- Keep merge history across reconnects, but require a fresh successful check
-- before reconciled panel settings replace settings read directly from a file.
ALTER TABLE config_file_connections ADD COLUMN initialization_required BOOLEAN NOT NULL DEFAULT TRUE;
