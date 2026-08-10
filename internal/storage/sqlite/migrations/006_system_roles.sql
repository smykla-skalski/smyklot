ALTER TABLE panel_users ADD COLUMN system_role TEXT NOT NULL DEFAULT 'none'
    CHECK (system_role IN ('none', 'root', 'super_root'));

UPDATE panel_users
SET system_role = CASE
    WHEN root = 1 THEN 'super_root'
    WHEN global_role = 'owner' AND status = 'active' THEN 'root'
    ELSE 'none'
END;

CREATE UNIQUE INDEX panel_users_single_super_root_idx
    ON panel_users (system_role)
    WHERE system_role = 'super_root';
