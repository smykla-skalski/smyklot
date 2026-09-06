package sqlstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// NotifyConfigFileChange records a source change without bypassing queue policy.
// Saving panel settings uses the same write inside its settings transaction.
func (s *Store) NotifyConfigFileChange(ctx context.Context, targetID, repositoryID string, now time.Time) error {
	if strings.TrimSpace(targetID) == "" || now.IsZero() {
		return errors.New("configuration change needs a target and time")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.lockInstallationSettingsTarget(ctx, tx, targetID); err != nil {
		return err
	}
	if err := notifyConfigFileChange(ctx, tx, targetID, repositoryID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func notifyConfigFileChange(ctx context.Context, tx *transaction, targetID, repositoryID string, now time.Time) error {
	enabled, _, err := configFileOwner(ctx, tx, targetID, repositoryID)
	if err != nil || !enabled {
		return err
	}
	var repository any
	if repositoryID != "" {
		repository = repositoryID
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO config_file_notifications (target_id, scope_key, repository_id, requested_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (target_id, scope_key) DO UPDATE SET requested_at = excluded.requested_at`,
		targetID, configFileScopeKey(repositoryID), repository, now)
	if err != nil {
		return fmt.Errorf("notify configuration file change: %w", err)
	}
	return nil
}

func notifyInstallationConfigFiles(ctx context.Context, tx *transaction, request storage.SaveInstallationSettingsRequest, work installationSettingsWork) error {
	if request.ConfigFileImport != nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, item := range work.items {
		if seen[item.RepositoryID] {
			continue
		}
		seen[item.RepositoryID] = true
		if err := notifyConfigFileChange(ctx, tx, request.TargetID, item.RepositoryID, request.ChangedAt); err != nil {
			return err
		}
	}
	return nil
}

type configFileNotification struct {
	targetID     string
	repositoryID string
}

// DispatchConfigFileNotifications consumes each notification in the transaction
// that makes its next check eligible. A running check keeps the notification for
// its successor, so a save during a GitHub request cannot be acknowledged early.
func (s *Store) DispatchConfigFileNotifications(ctx context.Context, now time.Time) (int, error) {
	if now.IsZero() {
		return 0, errors.New("configuration dispatch time is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := s.lockQueueDispatchState(ctx, tx, workqueue.LaneMaintenance); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT target_id, COALESCE(repository_id, '') FROM config_file_notifications
ORDER BY target_id, scope_key`+s.dialect.RowLock())
	if err != nil {
		return 0, err
	}
	notifications, err := collectRows(rows, func(row rowScanner) (configFileNotification, error) {
		var notification configFileNotification
		err := row.Scan(&notification.targetID, &notification.repositoryID)
		return notification, err
	})
	if err != nil {
		return 0, err
	}
	consumed := 0
	for _, notification := range notifications {
		done, err := s.dispatchConfigFileNotification(ctx, tx, notification, now)
		if err != nil {
			return 0, err
		}
		if done {
			consumed++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return consumed, nil
}

func (s *Store) dispatchConfigFileNotification(ctx context.Context, tx *transaction, notification configFileNotification, now time.Time) (bool, error) {
	enabled, _, err := configFileOwner(ctx, tx, notification.targetID, notification.repositoryID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return false, err
	}
	if enabled {
		claim := workqueue.RecurringClaim{
			Kind: workqueue.KindConfigFileSync, TargetID: &notification.targetID,
			Title: "Sync workspace configuration", Now: now, LeaseDuration: time.Minute,
		}
		if notification.repositoryID != "" {
			claim.RepositoryID, claim.Title = &notification.repositoryID, "Sync repository configuration"
		}
		item, err := s.ensureRecurringOccurrenceTx(ctx, tx, claim)
		if err != nil {
			return false, err
		}
		if item.ID == "" || item.State == workqueue.StateRunning {
			return false, nil
		}
		if err := s.expediteConfigFileCheck(ctx, tx, item, now); err != nil {
			return false, err
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM config_file_notifications WHERE target_id = ? AND scope_key = ?`,
		notification.targetID, configFileScopeKey(notification.repositoryID))
	return err == nil, err
}

func (s *Store) expediteConfigFileCheck(ctx context.Context, tx *transaction, item workqueue.Item, now time.Time) error {
	// Retrying and manually rescheduled work will read the new settings when it
	// runs. A notification must not defeat retry backoff or an operator's delay.
	if item.State == workqueue.StateRetrying || item.Immediate || !item.NotBefore.After(now) {
		return nil
	}
	overridden, err := recurringScheduleOverridden(ctx, tx, item.ID)
	if err != nil {
		return err
	}
	if overridden {
		return nil
	}
	policy, err := getEffectiveQueuePolicy(ctx, tx, item.Kind, item.TargetID)
	if err != nil {
		return err
	}
	profile, err := getScheduleProfile(ctx, tx, policy.ProfileID)
	if err != nil {
		return noRows(err)
	}
	eligible, err := workqueue.NextEligible(profile, now)
	if err != nil {
		return err
	}
	state := stateForEligibility(eligible, now)
	// This is a new source-driven check. Anchor its fallback here rather than
	// skipping the next interval from the occurrence's old future deadline.
	_, err = tx.ExecContext(ctx, `
UPDATE queue_items SET state = ?, profile_id = ?, not_before = ?, cadence_anchor_at = ?, eligible_at = ?,
    revision = revision + 1, updated_at = ? WHERE id = ?`,
		state, policy.ProfileID, now, now, eligible, now, item.ID)
	if err != nil {
		return err
	}
	return insertQueueEvent(ctx, tx, workqueue.Event{
		ItemID: item.ID, ActorID: queueEventActor(queueActorSystem), Kind: "configuration_changed",
		State: state, Summary: "Configuration changed", CreatedAt: now,
	})
}
