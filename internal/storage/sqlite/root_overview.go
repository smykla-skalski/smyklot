package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// GetRootOverview returns one consistent snapshot of Root operational counts.
func (s *Store) GetRootOverview(
	ctx context.Context,
	accountID string,
	now time.Time,
) (storage.RootOverview, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storage.RootOverview{}, fmt.Errorf("begin Root overview: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var result storage.RootOverview
	if err := readRootCatalogCounts(ctx, tx, &result); err != nil {
		return storage.RootOverview{}, err
	}
	if err := readRootOwnershipCounts(ctx, tx, now, &result); err != nil {
		return storage.RootOverview{}, err
	}
	if err := readRootSecurityCounts(ctx, tx, accountID, now, &result); err != nil {
		return storage.RootOverview{}, err
	}
	result.RecentFailures, err = readRootRecentFailures(ctx, tx)
	if err != nil {
		return storage.RootOverview{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.RootOverview{}, fmt.Errorf("commit Root overview: %w", err)
	}

	return result, nil
}

func readRootCatalogCounts(
	ctx context.Context,
	tx *sql.Tx,
	result *storage.RootOverview,
) error {
	if err := tx.QueryRowContext(ctx, `
SELECT
    COUNT(DISTINCT t.id),
    COUNT(r.id),
    COALESCE(SUM(CASE WHEN COALESCE(r.enabled_override, t.repository_default_enabled) = 1
                      THEN 1 ELSE 0 END), 0)
FROM targets t
LEFT JOIN repositories r ON r.target_id = t.id AND r.available = 1
WHERE t.available = 1`).Scan(
		&result.InstallationCount,
		&result.RepositoryCount,
		&result.EnabledRepositoryCount,
	); err != nil {
		return fmt.Errorf("read Root catalog counts: %w", err)
	}

	return nil
}

func readRootOwnershipCounts(
	ctx context.Context,
	tx *sql.Tx,
	now time.Time,
	result *storage.RootOverview,
) error {
	cutoff := formatTime(now.Add(-storage.OwnershipFreshFor))
	if err := tx.QueryRowContext(ctx, `
SELECT
    COALESCE(SUM(CASE WHEN o.status = 'fresh' AND o.synced_at >= ? AND
        (SELECT COUNT(*) FROM target_owners own WHERE own.target_id = t.id) > 0 THEN 1 ELSE 0 END), 0),
    COALESCE(SUM(CASE WHEN o.status IS NULL OR (o.status = 'fresh' AND (o.synced_at < ? OR
        (SELECT COUNT(*) FROM target_owners own WHERE own.target_id = t.id) = 0)) THEN 1 ELSE 0 END), 0),
    COALESCE(SUM(CASE WHEN o.status = 'permission_pending' THEN 1 ELSE 0 END), 0),
    COALESCE(SUM(CASE WHEN o.status = 'error' THEN 1 ELSE 0 END), 0)
FROM targets t
LEFT JOIN target_ownership o ON o.target_id = t.id
WHERE t.available = 1`, cutoff, cutoff).Scan(
		&result.OwnershipFresh,
		&result.OwnershipStale,
		&result.OwnershipPending,
		&result.OwnershipError,
	); err != nil {
		return fmt.Errorf("read Root ownership counts: %w", err)
	}

	return nil
}

func readRootSecurityCounts(
	ctx context.Context,
	tx *sql.Tx,
	accountID string,
	now time.Time,
	result *storage.RootOverview,
) error {
	if err := tx.QueryRowContext(ctx, `
SELECT
    (SELECT COUNT(*) FROM root_elevations WHERE ended_at IS NULL AND expires_at > ?),
    (SELECT COUNT(*) FROM security_notifications
     WHERE recipient_account_id = ? AND read_at IS NULL)`,
		formatTime(now), accountID,
	).Scan(&result.ActiveElevations, &result.UnreadSecurityEvents); err != nil {
		return fmt.Errorf("read Root security counts: %w", err)
	}

	return nil
}

func readRootRecentFailures(ctx context.Context, tx *sql.Tx) ([]storage.RootFailure, error) {
	rows, err := tx.QueryContext(
		ctx, rootFailureSelect+" WHERE d.status = ? ORDER BY d.id DESC LIMIT 5",
		storage.DeliveryFailed,
	)
	if err != nil {
		return nil, fmt.Errorf("list Root recent failures: %w", err)
	}
	items, err := collectRows(rows, scanRootFailure)
	if err != nil {
		return nil, fmt.Errorf("read Root recent failures: %w", err)
	}

	return items, nil
}

func scanRootFailure(scanner rowScanner) (storage.RootFailure, error) {
	var item storage.RootFailure
	var occurredAt, accountUpdatedAt string
	var avatar sql.NullString
	if err := scanner.Scan(
		&item.Failure.ID, &item.Failure.DeliveryID, &item.Failure.TargetID,
		&item.Failure.RepositoryFullName, &item.Failure.Event, &item.Failure.Stage,
		&item.Failure.Reason, &item.Failure.Retryable, &occurredAt,
		&item.Target.ID, &item.Target.Provider, &item.Target.SubjectID,
		&item.Target.Login, &item.Target.DisplayName, &avatar, &accountUpdatedAt,
	); err != nil {
		return storage.RootFailure{}, err
	}
	var err error
	item.Failure.OccurredAt, err = parseTime(occurredAt)
	if err != nil {
		return storage.RootFailure{}, err
	}
	item.Target.UpdatedAt, err = parseTime(accountUpdatedAt)
	item.Target.AvatarURL = stringPointer(avatar)

	return item, err
}
