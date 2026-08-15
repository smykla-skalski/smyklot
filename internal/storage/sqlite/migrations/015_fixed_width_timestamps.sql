-- Stored timestamps become fixed width.
--
-- SQLite has no timestamp type, so every comparison and every ORDER BY on a
-- stored time is a string comparison. Values were written with RFC3339Nano,
-- which drops trailing zeros from the fractional part, and that makes string
-- order and time order disagree: a whole second sorts after the same second
-- plus a fraction, because Z outranks the dot. Ordering by created_at and
-- expiring a session both read the wrong row when two values in the same
-- second carry different numbers of digits.
--
-- Padding the fraction to nine digits makes the two orders the same thing.
-- New writes already use that layout; these rewrite what is already stored.
--
-- Each statement is a no-op for a value that is already 30 characters long,
-- which for this layout means it already has all nine digits. The LIKE guard
-- leaves anything that is not a UTC RFC3339 timestamp untouched.

UPDATE access_audit_entries SET created_at =
    substr(created_at, 1, 19) || '.' || substr(
        CASE WHEN substr(created_at, 20, 1) = '.'
             THEN substr(created_at, 21, length(created_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE created_at IS NOT NULL
  AND length(created_at) <> 30
  AND created_at LIKE '____-__-__T__:__:__%Z';

UPDATE accounts SET updated_at =
    substr(updated_at, 1, 19) || '.' || substr(
        CASE WHEN substr(updated_at, 20, 1) = '.'
             THEN substr(updated_at, 21, length(updated_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE updated_at IS NOT NULL
  AND length(updated_at) <> 30
  AND updated_at LIKE '____-__-__T__:__:__%Z';

UPDATE app_audit_events SET created_at =
    substr(created_at, 1, 19) || '.' || substr(
        CASE WHEN substr(created_at, 20, 1) = '.'
             THEN substr(created_at, 21, length(created_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE created_at IS NOT NULL
  AND length(created_at) <> 30
  AND created_at LIKE '____-__-__T__:__:__%Z';

UPDATE audit_entries SET created_at =
    substr(created_at, 1, 19) || '.' || substr(
        CASE WHEN substr(created_at, 20, 1) = '.'
             THEN substr(created_at, 21, length(created_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE created_at IS NOT NULL
  AND length(created_at) <> 30
  AND created_at LIKE '____-__-__T__:__:__%Z';

UPDATE deliveries SET claimed_at =
    substr(claimed_at, 1, 19) || '.' || substr(
        CASE WHEN substr(claimed_at, 20, 1) = '.'
             THEN substr(claimed_at, 21, length(claimed_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE claimed_at IS NOT NULL
  AND length(claimed_at) <> 30
  AND claimed_at LIKE '____-__-__T__:__:__%Z';

UPDATE deliveries SET finished_at =
    substr(finished_at, 1, 19) || '.' || substr(
        CASE WHEN substr(finished_at, 20, 1) = '.'
             THEN substr(finished_at, 21, length(finished_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE finished_at IS NOT NULL
  AND length(finished_at) <> 30
  AND finished_at LIKE '____-__-__T__:__:__%Z';

UPDATE panel_users SET banned_at =
    substr(banned_at, 1, 19) || '.' || substr(
        CASE WHEN substr(banned_at, 20, 1) = '.'
             THEN substr(banned_at, 21, length(banned_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE banned_at IS NOT NULL
  AND length(banned_at) <> 30
  AND banned_at LIKE '____-__-__T__:__:__%Z';

UPDATE panel_users SET removed_at =
    substr(removed_at, 1, 19) || '.' || substr(
        CASE WHEN substr(removed_at, 20, 1) = '.'
             THEN substr(removed_at, 21, length(removed_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE removed_at IS NOT NULL
  AND length(removed_at) <> 30
  AND removed_at LIKE '____-__-__T__:__:__%Z';

UPDATE panel_users SET last_login_at =
    substr(last_login_at, 1, 19) || '.' || substr(
        CASE WHEN substr(last_login_at, 20, 1) = '.'
             THEN substr(last_login_at, 21, length(last_login_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE last_login_at IS NOT NULL
  AND length(last_login_at) <> 30
  AND last_login_at LIKE '____-__-__T__:__:__%Z';

UPDATE panel_users SET created_at =
    substr(created_at, 1, 19) || '.' || substr(
        CASE WHEN substr(created_at, 20, 1) = '.'
             THEN substr(created_at, 21, length(created_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE created_at IS NOT NULL
  AND length(created_at) <> 30
  AND created_at LIKE '____-__-__T__:__:__%Z';

UPDATE panel_users SET updated_at =
    substr(updated_at, 1, 19) || '.' || substr(
        CASE WHEN substr(updated_at, 20, 1) = '.'
             THEN substr(updated_at, 21, length(updated_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE updated_at IS NOT NULL
  AND length(updated_at) <> 30
  AND updated_at LIKE '____-__-__T__:__:__%Z';

UPDATE repositories SET settings_updated_at =
    substr(settings_updated_at, 1, 19) || '.' || substr(
        CASE WHEN substr(settings_updated_at, 20, 1) = '.'
             THEN substr(settings_updated_at, 21, length(settings_updated_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE settings_updated_at IS NOT NULL
  AND length(settings_updated_at) <> 30
  AND settings_updated_at LIKE '____-__-__T__:__:__%Z';

UPDATE repositories SET file_observed_at =
    substr(file_observed_at, 1, 19) || '.' || substr(
        CASE WHEN substr(file_observed_at, 20, 1) = '.'
             THEN substr(file_observed_at, 21, length(file_observed_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE file_observed_at IS NOT NULL
  AND length(file_observed_at) <> 30
  AND file_observed_at LIKE '____-__-__T__:__:__%Z';

UPDATE repositories SET synced_at =
    substr(synced_at, 1, 19) || '.' || substr(
        CASE WHEN substr(synced_at, 20, 1) = '.'
             THEN substr(synced_at, 21, length(synced_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE synced_at IS NOT NULL
  AND length(synced_at) <> 30
  AND synced_at LIKE '____-__-__T__:__:__%Z';

UPDATE root_elevations SET started_at =
    substr(started_at, 1, 19) || '.' || substr(
        CASE WHEN substr(started_at, 20, 1) = '.'
             THEN substr(started_at, 21, length(started_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE started_at IS NOT NULL
  AND length(started_at) <> 30
  AND started_at LIKE '____-__-__T__:__:__%Z';

UPDATE root_elevations SET expires_at =
    substr(expires_at, 1, 19) || '.' || substr(
        CASE WHEN substr(expires_at, 20, 1) = '.'
             THEN substr(expires_at, 21, length(expires_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE expires_at IS NOT NULL
  AND length(expires_at) <> 30
  AND expires_at LIKE '____-__-__T__:__:__%Z';

UPDATE root_elevations SET ended_at =
    substr(ended_at, 1, 19) || '.' || substr(
        CASE WHEN substr(ended_at, 20, 1) = '.'
             THEN substr(ended_at, 21, length(ended_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE ended_at IS NOT NULL
  AND length(ended_at) <> 30
  AND ended_at LIKE '____-__-__T__:__:__%Z';

UPDATE runtime_settings SET updated_at =
    substr(updated_at, 1, 19) || '.' || substr(
        CASE WHEN substr(updated_at, 20, 1) = '.'
             THEN substr(updated_at, 21, length(updated_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE updated_at IS NOT NULL
  AND length(updated_at) <> 30
  AND updated_at LIKE '____-__-__T__:__:__%Z';

UPDATE security_notifications SET created_at =
    substr(created_at, 1, 19) || '.' || substr(
        CASE WHEN substr(created_at, 20, 1) = '.'
             THEN substr(created_at, 21, length(created_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE created_at IS NOT NULL
  AND length(created_at) <> 30
  AND created_at LIKE '____-__-__T__:__:__%Z';

UPDATE security_notifications SET read_at =
    substr(read_at, 1, 19) || '.' || substr(
        CASE WHEN substr(read_at, 20, 1) = '.'
             THEN substr(read_at, 21, length(read_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE read_at IS NOT NULL
  AND length(read_at) <> 30
  AND read_at LIKE '____-__-__T__:__:__%Z';

UPDATE sessions SET created_at =
    substr(created_at, 1, 19) || '.' || substr(
        CASE WHEN substr(created_at, 20, 1) = '.'
             THEN substr(created_at, 21, length(created_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE created_at IS NOT NULL
  AND length(created_at) <> 30
  AND created_at LIKE '____-__-__T__:__:__%Z';

UPDATE sessions SET expires_at =
    substr(expires_at, 1, 19) || '.' || substr(
        CASE WHEN substr(expires_at, 20, 1) = '.'
             THEN substr(expires_at, 21, length(expires_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE expires_at IS NOT NULL
  AND length(expires_at) <> 30
  AND expires_at LIKE '____-__-__T__:__:__%Z';

UPDATE sessions SET revoked_at =
    substr(revoked_at, 1, 19) || '.' || substr(
        CASE WHEN substr(revoked_at, 20, 1) = '.'
             THEN substr(revoked_at, 21, length(revoked_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE revoked_at IS NOT NULL
  AND length(revoked_at) <> 30
  AND revoked_at LIKE '____-__-__T__:__:__%Z';

UPDATE target_owners SET synced_at =
    substr(synced_at, 1, 19) || '.' || substr(
        CASE WHEN substr(synced_at, 20, 1) = '.'
             THEN substr(synced_at, 21, length(synced_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE synced_at IS NOT NULL
  AND length(synced_at) <> 30
  AND synced_at LIKE '____-__-__T__:__:__%Z';

UPDATE target_ownership SET synced_at =
    substr(synced_at, 1, 19) || '.' || substr(
        CASE WHEN substr(synced_at, 20, 1) = '.'
             THEN substr(synced_at, 21, length(synced_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE synced_at IS NOT NULL
  AND length(synced_at) <> 30
  AND synced_at LIKE '____-__-__T__:__:__%Z';

UPDATE target_roles SET updated_at =
    substr(updated_at, 1, 19) || '.' || substr(
        CASE WHEN substr(updated_at, 20, 1) = '.'
             THEN substr(updated_at, 21, length(updated_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE updated_at IS NOT NULL
  AND length(updated_at) <> 30
  AND updated_at LIKE '____-__-__T__:__:__%Z';

UPDATE targets SET settings_updated_at =
    substr(settings_updated_at, 1, 19) || '.' || substr(
        CASE WHEN substr(settings_updated_at, 20, 1) = '.'
             THEN substr(settings_updated_at, 21, length(settings_updated_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE settings_updated_at IS NOT NULL
  AND length(settings_updated_at) <> 30
  AND settings_updated_at LIKE '____-__-__T__:__:__%Z';

UPDATE targets SET synced_at =
    substr(synced_at, 1, 19) || '.' || substr(
        CASE WHEN substr(synced_at, 20, 1) = '.'
             THEN substr(synced_at, 21, length(synced_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE synced_at IS NOT NULL
  AND length(synced_at) <> 30
  AND synced_at LIKE '____-__-__T__:__:__%Z';

UPDATE user_invitations SET expires_at =
    substr(expires_at, 1, 19) || '.' || substr(
        CASE WHEN substr(expires_at, 20, 1) = '.'
             THEN substr(expires_at, 21, length(expires_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE expires_at IS NOT NULL
  AND length(expires_at) <> 30
  AND expires_at LIKE '____-__-__T__:__:__%Z';

UPDATE user_invitations SET created_at =
    substr(created_at, 1, 19) || '.' || substr(
        CASE WHEN substr(created_at, 20, 1) = '.'
             THEN substr(created_at, 21, length(created_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE created_at IS NOT NULL
  AND length(created_at) <> 30
  AND created_at LIKE '____-__-__T__:__:__%Z';

UPDATE user_invitations SET responded_at =
    substr(responded_at, 1, 19) || '.' || substr(
        CASE WHEN substr(responded_at, 20, 1) = '.'
             THEN substr(responded_at, 21, length(responded_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE responded_at IS NOT NULL
  AND length(responded_at) <> 30
  AND responded_at LIKE '____-__-__T__:__:__%Z';

UPDATE user_preferences SET updated_at =
    substr(updated_at, 1, 19) || '.' || substr(
        CASE WHEN substr(updated_at, 20, 1) = '.'
             THEN substr(updated_at, 21, length(updated_at) - 21) || '000000000'
             ELSE '000000000' END, 1, 9) || 'Z'
WHERE updated_at IS NOT NULL
  AND length(updated_at) <> 30
  AND updated_at LIKE '____-__-__T__:__:__%Z';
