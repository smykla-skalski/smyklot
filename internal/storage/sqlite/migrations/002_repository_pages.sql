CREATE INDEX repositories_target_name_page_idx
    ON repositories (target_id, available, full_name COLLATE NOCASE, id);

CREATE INDEX repositories_target_updated_page_idx
    ON repositories (target_id, available, settings_updated_at, id);
