ALTER TABLE pending_ci_requests
ADD COLUMN pull_request_title TEXT NOT NULL DEFAULT '';
