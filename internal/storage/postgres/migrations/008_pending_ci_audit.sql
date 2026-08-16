ALTER TABLE pending_ci_requests
ADD COLUMN next_check_trigger TEXT NOT NULL DEFAULT 'fallback'
CHECK (next_check_trigger IN (
    'command', 'webhook', 'fallback', 'quiet_period', 'manual', 'cleanup'
));

UPDATE pending_ci_requests
SET next_check_trigger = 'cleanup'
WHERE cleanup_pending = TRUE;

CREATE TABLE pending_ci_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    request_id BIGINT NOT NULL REFERENCES pending_ci_requests(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN (
        'armed', 'superseded', 'wake_received', 'reconciliation_started',
        'checks_observed', 'merge_started', 'finished', 'cleanup_retry',
        'cleanup_completed'
    )),
    trigger_kind TEXT NOT NULL CHECK (trigger_kind IN (
        'command', 'webhook', 'fallback', 'quiet_period', 'manual', 'cleanup'
    )),
    event_name TEXT NOT NULL DEFAULT '',
    event_key TEXT NOT NULL DEFAULT '',
    delivery_id TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX pending_ci_events_request_idx
ON pending_ci_events (request_id, id DESC);
