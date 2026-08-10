ALTER TABLE user_invitations ADD COLUMN system_role TEXT
    CHECK (system_role IN ('root'));

INSERT INTO access_audit_entries (
    target_id, actor_account_id, subject_account_id, action, summary, created_at
)
SELECT
    NULL, created_by, account_id, 'invitation.migration_revoked',
    'revoked legacy global invitation during system role migration', created_at
FROM user_invitations
WHERE target_id IS NULL AND status = 'pending';

UPDATE user_invitations
SET status = 'revoked', responded_at = created_at
WHERE target_id IS NULL AND status = 'pending';

INSERT INTO app_audit_events (
    category, source_kind, source_id, actor_account_id, subject_account_id,
    action, summary, created_at
)
SELECT
    'access', 'access', id, actor_account_id, subject_account_id,
    action, summary, created_at
FROM access_audit_entries
WHERE action = 'invitation.migration_revoked'
  AND NOT EXISTS (
      SELECT 1 FROM app_audit_events
      WHERE source_kind = 'access' AND source_id = access_audit_entries.id
  );
