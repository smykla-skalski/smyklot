CREATE INDEX queue_items_page_idx
ON queue_items (
    (CASE WHEN finished_at IS NULL THEN 0 ELSE 1 END),
    (CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END),
    eligible_at,
    updated_at DESC,
    id,
    finished_at
);

CREATE INDEX queue_items_source_history_idx
ON queue_items (source_kind, source_id);

CREATE INDEX queue_items_source_open_idx
ON queue_items (source_kind, id)
WHERE state NOT IN ('succeeded', 'failed', 'cancelled', 'superseded');

CREATE INDEX queue_items_lane_finished_idx
ON queue_items (lane, finished_at DESC)
WHERE started_at IS NOT NULL AND finished_at IS NOT NULL;
