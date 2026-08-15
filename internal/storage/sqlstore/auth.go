package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// UpsertAccount records the current public profile for a stable provider
// identity.
func (s *Store) UpsertAccount(ctx context.Context, account storage.Account) error {
	if err := upsertAccount(ctx, s.db, account); err != nil {
		return fmt.Errorf("upsert account: %w", err)
	}

	return nil
}

// GetAccount returns one known panel identity.
func (s *Store) GetAccount(ctx context.Context, id string) (storage.Account, error) {
	account, err := getAccount(ctx, s.db, id)
	if err != nil {
		return storage.Account{}, fmt.Errorf("get account: %w", noRows(err))
	}

	return account, nil
}

// ReconcileSuperRoot makes accountID the singleton Super Root. Reassigning the
// configured identity demotes the former Super Root to Root atomically.
func (s *Store) ReconcileSuperRoot(
	ctx context.Context,
	accountID string,
	changedAt time.Time,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Super Root reconciliation: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var previousID string
	var previousStatus storage.PanelUserStatus
	previousErr := tx.QueryRowContext(ctx, `
SELECT account_id, status FROM panel_users WHERE system_role = 'super_root'`).
		Scan(&previousID, &previousStatus)
	if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
		return fmt.Errorf("read current Super Root: %w", previousErr)
	}
	if previousID == accountID && previousStatus == storage.PanelUserActive {
		return nil
	}

	if previousErr == nil && previousID != accountID {
		if _, err := tx.ExecContext(ctx, `
UPDATE panel_users
SET system_role = 'root', revision = revision + 1, updated_at = ?
WHERE account_id = ?`, changedAt, previousID); err != nil {
			return fmt.Errorf("demote former Super Root: %w", err)
		}
		if err := insertAccessAudit(
			ctx, tx, nil, accountID, previousID, "system_role.changed",
			"changed system role to root", changedAt,
		); err != nil {
			return err
		}
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO panel_users (
    account_id, status, revision, created_at, updated_at, system_role
)
SELECT id, 'active', 1, ?, ?, 'super_root'
FROM accounts WHERE id = ?
ON CONFLICT(account_id) DO UPDATE SET
    status = 'active',
    system_role = 'super_root',
    ban_reason = NULL,
    banned_at = NULL,
    removed_at = NULL,
    revision = panel_users.revision + 1,
    updated_at = excluded.updated_at`,
		changedAt,
		changedAt,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("promote configured Super Root: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read Super Root reconciliation result: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("promote configured Super Root: %w", storage.ErrNotFound)
	}
	if previousID != accountID {
		if err := insertAccessAudit(
			ctx, tx, nil, accountID, accountID, "system_role.changed",
			"changed system role to super_root", changedAt,
		); err != nil {
			return err
		}
	} else if err := insertAccessAudit(
		ctx, tx, nil, accountID, accountID, "system_role.restored",
		"restored configured Super Root", changedAt,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Super Root reconciliation: %w", err)
	}

	return nil
}

// ActivateDerivedOwner creates the regular panel identity for an account that
// has a fresh GitHub-derived Owner record. An existing lifecycle decision is
// never changed, so a ban or soft removal continues to override ownership.
func (s *Store) ActivateDerivedOwner(
	ctx context.Context,
	accountID string,
	changedAt time.Time,
) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin derived Owner activation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status storage.PanelUserStatus
	err = tx.QueryRowContext(
		ctx, "SELECT status FROM panel_users WHERE account_id = ?", accountID,
	).Scan(&status)
	if err == nil {
		return status == storage.PanelUserActive, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read derived Owner identity: %w", err)
	}
	var owned bool
	err = tx.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM target_owners owner
    JOIN targets t ON t.id = owner.target_id AND t.available = TRUE
    JOIN target_ownership ownership
      ON ownership.target_id = t.id AND ownership.status = 'fresh'
    WHERE owner.account_id = ?
      AND ownership.synced_at >= ?
      AND EXISTS(SELECT 1 FROM target_owners any_owner WHERE any_owner.target_id = t.id)
)`, accountID, changedAt.Add(-storage.OwnershipFreshFor)).Scan(&owned)
	if err != nil {
		return false, fmt.Errorf("resolve derived Owner identity: %w", err)
	}
	if !owned {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO panel_users (
    account_id, status, revision, created_at, updated_at, system_role
) VALUES (?, 'active', 1, ?, ?, 'none')`,
		accountID, changedAt, changedAt,
	)
	if err != nil {
		return false, fmt.Errorf("activate derived Owner: %w", s.conflictConstraint(err))
	}
	if err := insertAccessAudit(
		ctx, tx, nil, accountID, accountID, "owner.access.activated",
		"activated GitHub-derived Owner access", changedAt,
	); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit derived Owner activation: %w", err)
	}

	return true, nil
}

// CreateSession adds a session and keeps only the newest maxActive sessions
// for its account.
func (s *Store) CreateSession(ctx context.Context, session storage.Session, maxActive int) error {
	if maxActive < 1 {
		return errors.New("maximum active sessions must be positive")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session create: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	// Hold the account for the rest of the transaction. Capping its sessions
	// means reading which ones exist and then deleting the rest, and two
	// callers doing that at once would each read a set without the other's
	// row and delete nothing.
	var locked string
	if err := tx.QueryRowContext(
		ctx,
		"SELECT id FROM accounts WHERE id = ?"+s.dialect.RowLock(),
		session.AccountID,
	).Scan(&locked); err != nil {
		return fmt.Errorf("lock account for session create: %w", noRows(err))
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO sessions (token_hash, account_id, created_at, expires_at)
VALUES (?, ?, ?, ?)`,
		session.TokenHash,
		session.AccountID,
		session.CreatedAt,
		session.ExpiresAt,
	); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE panel_users SET last_login_at = ?, updated_at = ? WHERE account_id = ?`,
		session.CreatedAt,
		session.CreatedAt,
		session.AccountID,
	); err != nil {
		return fmt.Errorf("record panel login: %w", err)
	}

	// Keeping the newest rows says the same thing as skipping past them, and
	// says it without an unbounded LIMIT, which the two engines spell
	// differently.
	if _, err := tx.ExecContext(ctx, `
DELETE FROM sessions
WHERE account_id = ?
  AND token_hash NOT IN (
    SELECT token_hash FROM sessions
    WHERE account_id = ?
    ORDER BY created_at DESC, token_hash DESC
    LIMIT ?
)`, session.AccountID, session.AccountID, maxActive); err != nil {
		return fmt.Errorf("cap active sessions: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session create: %w", err)
	}

	return nil
}

// GetSession returns a live session by token digest.
func (s *Store) GetSession(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (storage.Session, error) {
	session, err := readSession(ctx, s.db, tokenHash)
	if err != nil {
		return storage.Session{}, fmt.Errorf("get session: %w", noRows(err))
	}

	if !now.Before(session.ExpiresAt) {
		if err := s.DeleteSession(ctx, tokenHash, storage.ElevationExpired, now); err != nil {
			return storage.Session{}, err
		}

		return storage.Session{}, storage.ErrExpired
	}
	if session.RevokedAt != nil {
		return session, storage.SessionRevokedError{
			Code:   valueOr(session.RevokeCode, "revoked"),
			Reason: valueOr(session.RevokeReason, "Your session was revoked"),
		}
	}

	return session, nil
}

// DeleteSession removes one session and terminates its Root elevation atomically.
func (s *Store) DeleteSession(
	ctx context.Context,
	tokenHash string,
	reason storage.ElevationEndReason,
	deletedAt time.Time,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := endSessionElevations(ctx, tx, tokenHash, reason, deletedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session delete: %w", err)
	}

	return nil
}

// RevokeAccountSessions preserves a safe reason until each session expires.
func (s *Store) RevokeAccountSessions(
	ctx context.Context,
	accountID, code, reason string,
	revokedAt time.Time,
) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin account session revocation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
SELECT token_hash FROM sessions
WHERE account_id = ? AND revoked_at IS NULL AND expires_at > ?
ORDER BY token_hash`, accountID, revokedAt)
	if err != nil {
		return nil, fmt.Errorf("list account sessions for revocation: %w", err)
	}
	hashes, err := collectRows(rows, func(scanner rowScanner) (string, error) {
		var hash string
		scanErr := scanner.Scan(&hash)

		return hash, scanErr
	})
	if err != nil {
		return nil, fmt.Errorf("read account sessions for revocation: %w", err)
	}
	for _, hash := range hashes {
		if err := endSessionElevations(ctx, tx, hash, storage.ElevationRevoked, revokedAt); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE sessions
SET revoked_at = ?, revoke_code = ?, revoke_reason = ?
WHERE account_id = ? AND revoked_at IS NULL AND expires_at > ?`,
		revokedAt,
		code,
		reason,
		accountID,
		revokedAt,
	); err != nil {
		return nil, fmt.Errorf("revoke account sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit account session revocation: %w", err)
	}

	return hashes, nil
}

// DeleteExpiredAuth records expired elevations and removes expired sessions.
func (s *Store) DeleteExpiredAuth(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin expired auth delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	expiredElevations, err := listExpiredElevations(ctx, tx, now)
	if err != nil {
		return err
	}
	for index := range expiredElevations {
		if err := endElevation(
			ctx, tx, &expiredElevations[index], storage.ElevationExpired, now,
		); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT token_hash FROM sessions WHERE expires_at <= ?", now)
	if err != nil {
		return fmt.Errorf("list expired sessions: %w", err)
	}
	hashes, err := collectRows(rows, func(scanner rowScanner) (string, error) {
		var hash string
		scanErr := scanner.Scan(&hash)

		return hash, scanErr
	})
	if err != nil {
		return fmt.Errorf("read expired sessions: %w", err)
	}
	for _, hash := range hashes {
		if err := endSessionElevations(ctx, tx, hash, storage.ElevationExpired, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", now); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit expired auth delete: %w", err)
	}

	return nil
}

func upsertAccount(ctx context.Context, executor runner, account storage.Account) error {
	_, err := executor.ExecContext(ctx, `
INSERT INTO accounts (id, provider, subject_id, login, display_name, avatar_url, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    provider = excluded.provider,
    subject_id = excluded.subject_id,
    login = excluded.login,
    display_name = excluded.display_name,
    avatar_url = excluded.avatar_url,
    updated_at = excluded.updated_at`,
		account.ID,
		account.Provider,
		account.SubjectID,
		account.Login,
		account.DisplayName,
		account.AvatarURL,
		account.UpdatedAt,
	)

	return err
}

func getAccount(ctx context.Context, queryer rowQuerier, id string) (storage.Account, error) {
	var account storage.Account
	var avatarURL sql.NullString
	var updatedAt StoredTime

	err := queryer.QueryRowContext(ctx, `
SELECT id, provider, subject_id, login, display_name, avatar_url, updated_at
FROM accounts WHERE id = ?`, id).Scan(
		&account.ID,
		&account.Provider,
		&account.SubjectID,
		&account.Login,
		&account.DisplayName,
		&avatarURL,
		&updatedAt,
	)
	if err != nil {
		return storage.Account{}, err
	}

	account.AvatarURL = stringPointer(avatarURL)
	account.UpdatedAt = updatedAt.Time()

	return account, nil
}

func readSession(
	ctx context.Context,
	queryer rowQuerier,
	tokenHash string,
) (storage.Session, error) {
	var session storage.Session

	var revokeCode, revokeReason sql.NullString
	var revokedAt StoredTime
	times, err := scanTimeRange(queryer.QueryRowContext(ctx, `
SELECT token_hash, account_id, revoked_at, revoke_code, revoke_reason, created_at, expires_at
FROM sessions WHERE token_hash = ?`, tokenHash),
		&session.TokenHash,
		&session.AccountID,
		&revokedAt,
		&revokeCode,
		&revokeReason,
	)
	if err != nil {
		return storage.Session{}, err
	}

	session.CreatedAt = times.createdAt
	session.ExpiresAt = times.expiresAt
	session.RevokedAt = revokedAt.Pointer()
	session.RevokeCode = stringPointer(revokeCode)
	session.RevokeReason = stringPointer(revokeReason)

	return session, nil
}

func valueOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}

	return *value
}
