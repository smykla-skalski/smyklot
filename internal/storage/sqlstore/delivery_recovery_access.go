package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// Lock the records that can revoke this decision through the recovery commit.
func (s *Store) authorizeDeliveryRecovery(ctx context.Context, tx *transaction, request storage.DeliveryRecovery) error {
	for _, lock := range []struct {
		query string
		args  []any
	}{
		{"SELECT account_id FROM panel_users WHERE account_id = ?", []any{request.ActorAccountID}},
		{"SELECT token_hash FROM sessions WHERE token_hash = ?", []any{request.SessionTokenHash}},
		{"SELECT id FROM targets WHERE id = ?", []any{request.TargetID}},
		{"SELECT target_id FROM target_ownership WHERE target_id = ?", []any{request.TargetID}},
		{"SELECT account_id FROM target_owners WHERE target_id = ?", []any{request.TargetID}},
		{"SELECT account_id FROM target_roles WHERE target_id = ? AND account_id = ?", []any{request.TargetID, request.ActorAccountID}},
	} {
		rows, err := tx.QueryContext(ctx, lock.query+s.dialect.RowLock(), lock.args...)
		if err != nil {
			return fmt.Errorf("lock delivery recovery access: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
	}
	var accountID string
	var expiresAt, revokedAt StoredTime
	if err := tx.QueryRowContext(ctx, "SELECT account_id, expires_at, revoked_at FROM sessions WHERE token_hash = ?", request.SessionTokenHash).Scan(&accountID, &expiresAt, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.ErrRevoked
		}
		return fmt.Errorf("read recovery session: %w", err)
	}
	if accountID != request.ActorAccountID || revokedAt.Valid() || !request.RequestedAt.Before(expiresAt.Time()) {
		return storage.ErrRevoked
	}
	access, err := resolveTargetAccess(ctx, tx, request.ActorAccountID, request.TargetID, request.RequestedAt)
	if err != nil {
		return err
	}
	if request.ElevationID != nil {
		_, err := s.elevatedWrite(ctx, tx, request.ElevationID, request.SessionTokenHash, request.ActorAccountID, request.TargetID, request.RequestedAt)
		return err
	}
	if access.Role != storage.InstallationRoleOwner && access.Role != storage.InstallationRoleAdmin {
		return storage.ErrRevoked
	}
	return nil
}
