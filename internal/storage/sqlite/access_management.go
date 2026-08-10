package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// ListPanelUsers returns every current panel user in stable login order.
func (s *Store) ListPanelUsers(ctx context.Context) ([]storage.PanelUser, error) {
	rows, err := s.db.QueryContext(ctx, panelUserSelect+`
WHERE pu.status <> 'removed'
ORDER BY lower(a.login), a.id`)
	if err != nil {
		return nil, fmt.Errorf("list panel users: %w", err)
	}
	users, err := collectRows(rows, scanPanelUser)
	if err != nil {
		return nil, fmt.Errorf("read panel users: %w", err)
	}

	return users, nil
}

// ListTargetPanelUsers returns users whose policy is relevant to one available
// installation.
func (s *Store) ListTargetPanelUsers(
	ctx context.Context,
	targetID string,
	now time.Time,
) ([]storage.TargetPanelUser, error) {
	var available bool
	err := s.db.QueryRowContext(ctx, "SELECT available FROM targets WHERE id = ?", targetID).
		Scan(&available)
	if err != nil {
		return nil, fmt.Errorf("read target users target: %w", noRows(err))
	}
	if !available {
		return nil, fmt.Errorf("read target users target: %w", storage.ErrNotFound)
	}
	users, err := s.ListPanelUsers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]storage.TargetPanelUser, 0, len(users))
	for _, user := range users {
		override, overrideErr := getTargetAccessOverride(ctx, s.db, user.Account.ID, targetID)
		var overridePointer *storage.TargetAccessOverride
		if overrideErr == nil {
			overridePointer = &override
		} else if !errors.Is(overrideErr, sql.ErrNoRows) {
			return nil, fmt.Errorf("read target user override: %w", overrideErr)
		}
		access, accessErr := s.ResolveTargetAccess(ctx, user.Account.ID, targetID, now)
		if accessErr != nil {
			return nil, accessErr
		}
		if overridePointer == nil && access.Role == storage.InstallationRoleNone {
			continue
		}
		result = append(result, storage.TargetPanelUser{
			User: user, Override: overridePointer, Access: access,
		})
	}

	return result, nil
}

// UpdatePanelUser replaces an account lifecycle state with optimistic
// concurrency and an audit entry in the same transaction.
func (s *Store) UpdatePanelUser(
	ctx context.Context,
	change storage.PanelUserChange,
) (storage.PanelUser, error) {
	if !validPanelUserStatus(change.Status) {
		return storage.PanelUser{}, errors.New("unsupported panel user policy")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("begin panel user update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := getPanelUser(ctx, tx, change.AccountID)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("read panel user for update: %w", noRows(err))
	}
	if current.Revision != change.ExpectedRevision {
		return storage.PanelUser{}, storage.ErrConflict
	}
	if current.SystemRole.IsRoot() && change.Status != storage.PanelUserActive {
		return storage.PanelUser{}, storage.ErrConflict
	}
	if current.Status == storage.PanelUserRemoved {
		return storage.PanelUser{}, storage.ErrConflict
	}

	values := panelUserUpdateValues(current, change)
	result, err := tx.ExecContext(ctx, `
UPDATE panel_users
SET status = ?, ban_reason = ?, banned_at = ?, removed_at = ?,
    revision = revision + 1, updated_at = ?
WHERE account_id = ? AND revision = ?`,
		change.Status,
		values.banReason,
		values.bannedAt,
		values.removedAt,
		formatTime(change.ChangedAt),
		change.AccountID,
		change.ExpectedRevision,
	)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("update panel user: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("read panel user update result: %w", err)
	}
	if changed != 1 {
		return storage.PanelUser{}, storage.ErrConflict
	}
	if change.Status == storage.PanelUserRemoved {
		if _, err := tx.ExecContext(ctx, `
UPDATE user_invitations
SET status = 'revoked', responded_at = ?
WHERE account_id = ? AND status = 'pending'`,
			formatTime(change.ChangedAt), change.AccountID,
		); err != nil {
			return storage.PanelUser{}, fmt.Errorf("revoke removed user invitations: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM target_roles WHERE account_id = ?", change.AccountID); err != nil {
			return storage.PanelUser{}, fmt.Errorf("delete removed user target roles: %w", err)
		}
	}
	if err := insertAccessAudit(
		ctx,
		tx,
		nil,
		change.ActorAccountID,
		change.AccountID,
		values.action,
		values.summary,
		change.ChangedAt,
	); err != nil {
		return storage.PanelUser{}, err
	}
	updated, err := getPanelUser(ctx, tx, change.AccountID)
	if err != nil {
		return storage.PanelUser{}, fmt.Errorf("read updated panel user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return storage.PanelUser{}, fmt.Errorf("commit panel user update: %w", err)
	}

	return updated, nil
}

type panelUserUpdate struct {
	banReason any
	bannedAt  any
	removedAt any
	action    string
	summary   string
}

func panelUserUpdateValues(
	current storage.PanelUser,
	change storage.PanelUserChange,
) panelUserUpdate {
	values := panelUserUpdate{
		action: "user.restored", summary: "restored user",
	}
	switch change.Status {
	case storage.PanelUserActive:
		if current.Status == storage.PanelUserBanned {
			values.action = "user.unbanned"
			values.summary = "unbanned user"
		}
	case storage.PanelUserBanned:
		values.banReason = normalizedOptional(change.BanReason)
		values.bannedAt = formatTime(change.ChangedAt)
		values.action = "user.banned"
		values.summary = "banned user"
		if reason, ok := values.banReason.(string); ok {
			values.summary += ": " + reason
		}
	case storage.PanelUserRemoved:
		values.removedAt = formatTime(change.ChangedAt)
		values.action = "user.removed"
		values.summary = "removed user"
	}

	return values
}

func validPanelUserStatus(status storage.PanelUserStatus) bool {
	switch status {
	case storage.PanelUserActive, storage.PanelUserBanned, storage.PanelUserRemoved:
		return true
	default:
		return false
	}
}

func normalizedOptional(value *string) any {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}

	return normalized
}
