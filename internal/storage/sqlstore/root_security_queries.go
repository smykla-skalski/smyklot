package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const elevationSelect = `
SELECT
    id, session_token_hash, root_account_id, target_id, reason,
    started_at, expires_at, ended_at, end_reason
FROM root_elevations`

func getElevationByID(
	ctx context.Context,
	queryer rowQuerier,
	id, sessionTokenHash string,
) (storage.Elevation, error) {
	return scanElevation(queryer.QueryRowContext(
		ctx,
		elevationSelect+" WHERE id = ? AND session_token_hash = ?",
		id,
		sessionTokenHash,
	))
}

func getElevationByIDForWrite(
	ctx context.Context,
	queryer rowQuerier,
	dialect Dialect,
	id, sessionTokenHash string,
) (storage.Elevation, error) {
	return scanElevation(queryer.QueryRowContext(
		ctx,
		elevationSelect+" WHERE id = ? AND session_token_hash = ?"+dialect.RowLock(),
		id,
		sessionTokenHash,
	))
}

func getElevationBySessionTarget(
	ctx context.Context,
	queryer rowQuerier,
	sessionTokenHash, targetID string,
) (storage.Elevation, error) {
	return scanElevation(queryer.QueryRowContext(ctx, elevationSelect+`
WHERE session_token_hash = ? AND target_id = ?
ORDER BY started_at DESC LIMIT 1`, sessionTokenHash, targetID))
}

func listOpenSessionElevations(
	ctx context.Context,
	queryer interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	sessionTokenHash string,
) ([]storage.Elevation, error) {
	rows, err := queryer.QueryContext(ctx, elevationSelect+`
WHERE session_token_hash = ? AND ended_at IS NULL
ORDER BY started_at`, sessionTokenHash)
	if err != nil {
		return nil, fmt.Errorf("list open session elevations: %w", err)
	}
	elevations, err := collectRows(rows, scanElevation)
	if err != nil {
		return nil, fmt.Errorf("read open session elevations: %w", err)
	}

	return elevations, nil
}

func listExpiredElevations(
	ctx context.Context,
	queryer interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	now time.Time,
) ([]storage.Elevation, error) {
	rows, err := queryer.QueryContext(ctx, elevationSelect+`
WHERE ended_at IS NULL AND expires_at <= ?
ORDER BY expires_at`, now)
	if err != nil {
		return nil, fmt.Errorf("list expired elevations: %w", err)
	}
	elevations, err := collectRows(rows, scanElevation)
	if err != nil {
		return nil, fmt.Errorf("read expired elevations: %w", err)
	}

	return elevations, nil
}

func scanElevation(scanner rowScanner) (storage.Elevation, error) {
	var elevation storage.Elevation
	var reason, endReason sql.NullString
	var endedAt, startedAt, expiresAt StoredTime
	if err := scanner.Scan(
		&elevation.ID,
		&elevation.SessionTokenHash,
		&elevation.RootAccountID,
		&elevation.TargetID,
		&reason,
		&startedAt,
		&expiresAt,
		&endedAt,
		&endReason,
	); err != nil {
		return storage.Elevation{}, err
	}
	elevation.Reason = stringPointer(reason)
	elevation.StartedAt = startedAt.Time()
	elevation.ExpiresAt = expiresAt.Time()
	elevation.EndedAt = endedAt.Pointer()
	if endReason.Valid {
		parsed := storage.ElevationEndReason(endReason.String)
		elevation.EndReason = &parsed
	}

	return elevation, nil
}

// ListSecurityNotifications returns newest Owner notices and unread totals.
func (s *Store) ListSecurityNotifications(
	ctx context.Context,
	recipientAccountID string,
	page storage.NotificationPageRequest,
) (storage.NotificationPage, error) {
	limit := pageLimit(page.Limit)
	offset := max(page.Offset, 0)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storage.NotificationPage{}, fmt.Errorf("begin notification page: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var total, unread int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*), COUNT(*) FILTER (WHERE read_at IS NULL)
FROM security_notifications WHERE recipient_account_id = ?`, recipientAccountID).
		Scan(&total, &unread); err != nil {
		return storage.NotificationPage{}, fmt.Errorf("count security notifications: %w", err)
	}
	rows, err := tx.QueryContext(
		ctx,
		notificationSelect+`
WHERE n.recipient_account_id = ?
ORDER BY n.id DESC LIMIT ? OFFSET ?`,
		recipientAccountID,
		limit+1,
		offset,
	)
	if err != nil {
		return storage.NotificationPage{}, fmt.Errorf("list security notifications: %w", err)
	}
	items, err := collectRows(rows, scanSecurityNotification)
	if err != nil {
		return storage.NotificationPage{}, fmt.Errorf("read security notifications: %w", err)
	}
	result := storage.NotificationPage{Items: items, Total: total, Unread: unread}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.NextOffset = offset + limit
	}
	if err := tx.Commit(); err != nil {
		return storage.NotificationPage{}, fmt.Errorf("commit notification page: %w", err)
	}

	return result, nil
}

// MarkSecurityNotificationRead records per-recipient read state idempotently.
func (s *Store) MarkSecurityNotificationRead(
	ctx context.Context,
	recipientAccountID string,
	notificationID int64,
	readAt time.Time,
) (storage.SecurityNotification, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SecurityNotification{}, fmt.Errorf("begin notification read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
UPDATE security_notifications SET read_at = ?
WHERE id = ? AND recipient_account_id = ? AND read_at IS NULL`,
		readAt, notificationID, recipientAccountID,
	); err != nil {
		return storage.SecurityNotification{}, fmt.Errorf("mark security notification read: %w", err)
	}
	notification, err := getSecurityNotification(ctx, tx, recipientAccountID, notificationID)
	if err != nil {
		return storage.SecurityNotification{}, fmt.Errorf("read security notification: %w", noRows(err))
	}
	if err := tx.Commit(); err != nil {
		return storage.SecurityNotification{}, fmt.Errorf("commit notification read: %w", err)
	}

	return notification, nil
}

const notificationSelect = `
SELECT
    n.id, n.recipient_account_id, n.target_id, n.elevation_id, n.audit_event_id,
    n.action, n.reason, n.created_at, n.read_at,
    target_account.id, target_account.provider, target_account.subject_id,
    target_account.login, target_account.display_name, target_account.avatar_url,
    target_account.updated_at,
    actor.id, actor.provider, actor.subject_id, actor.login, actor.display_name,
    actor.avatar_url, actor.updated_at
FROM security_notifications n
JOIN targets target ON target.id = n.target_id
JOIN accounts target_account ON target_account.id = target.account_id
JOIN accounts actor ON actor.id = n.actor_account_id`

func getSecurityNotification(
	ctx context.Context,
	queryer rowQuerier,
	recipientAccountID string,
	notificationID int64,
) (storage.SecurityNotification, error) {
	return scanSecurityNotification(queryer.QueryRowContext(
		ctx,
		notificationSelect+" WHERE n.recipient_account_id = ? AND n.id = ?",
		recipientAccountID,
		notificationID,
	))
}

func scanSecurityNotification(scanner rowScanner) (storage.SecurityNotification, error) {
	var notification storage.SecurityNotification
	var reason, targetAvatar, actorAvatar sql.NullString
	var readAt, createdAt, targetUpdatedAt, actorUpdatedAt StoredTime
	if err := scanner.Scan(
		&notification.ID,
		&notification.RecipientID,
		&notification.TargetID,
		&notification.ElevationID,
		&notification.AuditEventID,
		&notification.Action,
		&reason,
		&createdAt,
		&readAt,
		&notification.Target.ID,
		&notification.Target.Provider,
		&notification.Target.SubjectID,
		&notification.Target.Login,
		&notification.Target.DisplayName,
		&targetAvatar,
		&targetUpdatedAt,
		&notification.Actor.ID,
		&notification.Actor.Provider,
		&notification.Actor.SubjectID,
		&notification.Actor.Login,
		&notification.Actor.DisplayName,
		&actorAvatar,
		&actorUpdatedAt,
	); err != nil {
		return storage.SecurityNotification{}, err
	}
	notification.Reason = stringPointer(reason)
	notification.Target.AvatarURL = stringPointer(targetAvatar)
	notification.Actor.AvatarURL = stringPointer(actorAvatar)
	notification.CreatedAt = createdAt.Time()
	notification.ReadAt = readAt.Pointer()
	notification.Target.UpdatedAt = targetUpdatedAt.Time()
	notification.Actor.UpdatedAt = actorUpdatedAt.Time()

	return notification, nil
}
