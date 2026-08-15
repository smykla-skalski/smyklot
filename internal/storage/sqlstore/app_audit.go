package sqlstore

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

type appAuditInsert struct {
	Category         string
	SourceKind       *string
	SourceID         *int64
	TargetID         *string
	ActorAccountID   string
	SubjectAccountID *string
	ElevationID      *string
	Action           string
	Summary          string
	CreatedAt        time.Time
}

func insertAppAudit(
	ctx context.Context,
	executor runner,
	entry appAuditInsert,
) (int64, error) {
	var id int64
	err := executor.QueryRowContext(ctx, `
INSERT INTO app_audit_events (
    category, source_kind, source_id, target_id, actor_account_id,
    subject_account_id, elevation_id, action, summary, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		entry.Category,
		entry.SourceKind,
		entry.SourceID,
		entry.TargetID,
		entry.ActorAccountID,
		entry.SubjectAccountID,
		entry.ElevationID,
		entry.Action,
		entry.Summary,
		entry.CreatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert app audit: %w", err)
	}

	return id, nil
}

func insertElevatedNotifications(
	ctx context.Context,
	tx runner,
	elevation storage.Elevation,
	auditEventID int64,
	action string,
	createdAt time.Time,
) error {
	rows, err := tx.QueryContext(ctx, `
SELECT account_id
FROM target_owners
WHERE target_id = ?
ORDER BY account_id`, elevation.TargetID)
	if err != nil {
		return fmt.Errorf("list notification recipients: %w", err)
	}
	owners, err := collectRows(rows, func(scanner rowScanner) (string, error) {
		var accountID string
		scanErr := scanner.Scan(&accountID)

		return accountID, scanErr
	})
	if err != nil {
		return fmt.Errorf("read notification recipients: %w", err)
	}
	if len(owners) == 0 {
		return fmt.Errorf("insert Owner notifications: %w", storage.ErrConflict)
	}
	for _, ownerID := range owners {
		if err := insertElevatedNotification(
			ctx, tx, elevation, auditEventID, action, ownerID, createdAt,
		); err != nil {
			return err
		}
	}

	return nil
}

func insertElevatedNotification(
	ctx context.Context,
	tx runner,
	elevation storage.Elevation,
	auditEventID int64,
	action, ownerID string,
	createdAt time.Time,
) error {
	var notificationID int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO security_notifications (
    recipient_account_id, target_id, actor_account_id, elevation_id,
    audit_event_id, action, reason, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		ownerID,
		elevation.TargetID,
		elevation.RootAccountID,
		elevation.ID,
		auditEventID,
		action,
		elevation.Reason,
		createdAt,
	).Scan(&notificationID)
	if err != nil {
		return fmt.Errorf("insert Owner notifications: %w", err)
	}
	sourceKind := "notification"
	targetID := elevation.TargetID
	if _, err := insertAppAudit(ctx, tx, appAuditInsert{
		Category:         string(storage.AuditCategoryNotification),
		SourceKind:       &sourceKind,
		SourceID:         &notificationID,
		TargetID:         &targetID,
		ActorAccountID:   elevation.RootAccountID,
		SubjectAccountID: &ownerID,
		ElevationID:      &elevation.ID,
		Action:           "owner.notification.created",
		Summary:          "Notified installation Owner about " + action,
		CreatedAt:        createdAt,
	}); err != nil {
		return fmt.Errorf("record Owner notification: %w", err)
	}

	return nil
}
