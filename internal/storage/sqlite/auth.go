package sqlite

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

// ClaimOwner binds the panel to the first approved account. The binding is
// immutable: later calls return whether the supplied account already owns it.
func (s *Store) ClaimOwner(ctx context.Context, accountID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin panel owner claim: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO panel_owner (singleton, account_id)
VALUES (1, ?)
ON CONFLICT(singleton) DO NOTHING`, accountID); err != nil {
		return false, fmt.Errorf("claim panel owner: %w", err)
	}

	var ownerID string
	if err := tx.QueryRowContext(ctx, "SELECT account_id FROM panel_owner WHERE singleton = 1").
		Scan(&ownerID); err != nil {
		return false, fmt.Errorf("read panel owner claim: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO panel_users (
    account_id, root, status, global_role, revision, created_at, updated_at
)
SELECT id, 1, 'active', 'owner', 1, updated_at, updated_at
FROM accounts WHERE id = ?
ON CONFLICT(account_id) DO UPDATE SET
    root = 1,
    status = 'active',
    global_role = 'owner',
    ban_reason = NULL,
    banned_at = NULL,
    removed_at = NULL`, ownerID); err != nil {
		return false, fmt.Errorf("activate panel root: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit panel owner claim: %w", err)
	}

	return ownerID == accountID, nil
}

// IsOwner reports whether the immutable panel owner binding names accountID.
func (s *Store) IsOwner(ctx context.Context, accountID string) (bool, error) {
	var ownerID string
	err := s.db.QueryRowContext(ctx, "SELECT account_id FROM panel_owner WHERE singleton = 1").
		Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read panel owner: %w", err)
	}

	return ownerID == accountID, nil
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

	if _, err := tx.ExecContext(ctx, `
INSERT INTO sessions (token_hash, account_id, created_at, expires_at)
VALUES (?, ?, ?, ?)`,
		session.TokenHash,
		session.AccountID,
		formatTime(session.CreatedAt),
		formatTime(session.ExpiresAt),
	); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE panel_users SET last_login_at = ?, updated_at = ? WHERE account_id = ?`,
		formatTime(session.CreatedAt),
		formatTime(session.CreatedAt),
		session.AccountID,
	); err != nil {
		return fmt.Errorf("record panel login: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
DELETE FROM sessions
WHERE token_hash IN (
    SELECT token_hash FROM sessions
    WHERE account_id = ?
    ORDER BY created_at DESC, token_hash DESC
    LIMIT -1 OFFSET ?
)`, session.AccountID, maxActive); err != nil {
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
		if err := s.DeleteSession(ctx, tokenHash); err != nil {
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

// DeleteSession revokes one session. It is idempotent.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
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
ORDER BY token_hash`, accountID, formatTime(revokedAt))
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
	if _, err := tx.ExecContext(ctx, `
UPDATE sessions
SET revoked_at = ?, revoke_code = ?, revoke_reason = ?
WHERE account_id = ? AND revoked_at IS NULL AND expires_at > ?`,
		formatTime(revokedAt),
		code,
		reason,
		accountID,
		formatTime(revokedAt),
	); err != nil {
		return nil, fmt.Errorf("revoke account sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit account session revocation: %w", err)
	}

	return hashes, nil
}

// DeleteExpiredAuth removes expired sessions.
func (s *Store) DeleteExpiredAuth(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", formatTime(now)); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}

	return nil
}

type accountExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func upsertAccount(ctx context.Context, executor accountExecutor, account storage.Account) error {
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
		formatTime(account.UpdatedAt),
	)

	return err
}

func getAccount(ctx context.Context, queryer rowQuerier, id string) (storage.Account, error) {
	var account storage.Account
	var avatarURL sql.NullString
	var updatedAt string

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
	account.UpdatedAt, err = parseTime(updatedAt)

	return account, err
}

func readSession(
	ctx context.Context,
	queryer rowQuerier,
	tokenHash string,
) (storage.Session, error) {
	var session storage.Session

	var revokedAt, revokeCode, revokeReason sql.NullString
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
	session.RevokedAt, err = nullableTime(revokedAt)
	if err != nil {
		return storage.Session{}, err
	}
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
