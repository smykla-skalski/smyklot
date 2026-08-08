package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// ClaimDelivery atomically accepts an event revision once and distinguishes a
// still-running attempt from a retained terminal outcome.
func (s *Store) ClaimDelivery(
	ctx context.Context,
	claim storage.DeliveryClaim,
) (storage.DeliveryClaimResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("begin delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
	INSERT INTO deliveries (
    claim_key, delivery_id, target_id, repository_id, repository_full_name,
    event, status, claimed_at
)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING`,
		claim.ClaimKey,
		claim.DeliveryID,
		claim.TargetID,
		claim.RepositoryID,
		claim.RepositoryFullName,
		claim.Event,
		storage.DeliveryRunning,
		formatTime(claim.ClaimedAt),
	)
	if err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("claim delivery: %w", err)
	}

	changed, err := result.RowsAffected()
	if err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("read delivery claim result: %w", err)
	}
	if changed == 0 {
		var status storage.DeliveryStatus
		if err := tx.QueryRowContext(ctx, `
	SELECT status FROM deliveries
	WHERE claim_key = ?
	  AND (status IN (?, ?) OR (status = ? AND retryable = 0))`,
			claim.ClaimKey,
			storage.DeliveryRunning,
			storage.DeliverySucceeded,
			storage.DeliveryFailed,
		).Scan(&status); err != nil {
			return storage.DeliveryClaimResult{}, fmt.Errorf("read retained delivery claim: %w", err)
		}
		disposition := storage.DeliveryClaimRetained
		if status == storage.DeliveryRunning {
			disposition = storage.DeliveryClaimInProgress
		}
		if err := tx.Commit(); err != nil {
			return storage.DeliveryClaimResult{}, fmt.Errorf("commit duplicate delivery claim: %w", err)
		}

		return storage.DeliveryClaimResult{Disposition: disposition}, nil
	}
	claimID, err := result.LastInsertId()
	if err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("read delivery claim id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("commit delivery claim: %w", err)
	}

	return storage.DeliveryClaimResult{
		ID:          claimID,
		Disposition: storage.DeliveryClaimAccepted,
	}, nil
}

// AbandonDelivery releases a running claim that never entered execution, such
// as a delivery refused because the bounded worker queue was full.
func (s *Store) AbandonDelivery(ctx context.Context, claimID int64) error {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM deliveries WHERE id = ? AND status = ?`,
		claimID,
		storage.DeliveryRunning,
	)
	if err != nil {
		return fmt.Errorf("abandon delivery: %w", err)
	}

	return s.checkDeliveryUpdate(ctx, result, claimID)
}

// CompleteDelivery marks a running delivery successful. Repeating the same
// outcome is safe when a caller lost the first database result and retries.
func (s *Store) CompleteDelivery(
	ctx context.Context,
	claimID int64,
	completedAt time.Time,
) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE deliveries SET status = ?, finished_at = ?
WHERE id = ? AND status IN (?, ?)`,
		storage.DeliverySucceeded,
		formatTime(completedAt),
		claimID,
		storage.DeliveryRunning,
		storage.DeliverySucceeded,
	)
	if err != nil {
		return fmt.Errorf("complete delivery: %w", err)
	}

	return s.checkDeliveryUpdate(ctx, result, claimID)
}

// FailDelivery marks a running delivery failed with a sanitized reason.
// Repeating the same outcome is safe when finalization is retried.
func (s *Store) FailDelivery(
	ctx context.Context,
	change storage.DeliveryFailureChange,
) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE deliveries SET
    status = ?,
    stage = ?,
    reason = ?,
    retryable = ?,
    finished_at = ?
WHERE id = ? AND status IN (?, ?)`,
		storage.DeliveryFailed,
		change.Stage,
		change.Reason,
		change.Retryable,
		formatTime(change.FailedAt),
		change.ClaimID,
		storage.DeliveryRunning,
		storage.DeliveryFailed,
	)
	if err != nil {
		return fmt.Errorf("fail delivery: %w", err)
	}

	return s.checkDeliveryUpdate(ctx, result, change.ClaimID)
}

// RecoverRunningDeliveries releases claims that belonged to the previous
// process. The deployment is intentionally single-replica, so no running row
// can still have an executor when a new store owner starts.
func (s *Store) RecoverRunningDeliveries(ctx context.Context, recoveredAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE deliveries SET
    status = ?,
    stage = ?,
    reason = ?,
    retryable = 1,
    finished_at = ?
WHERE status = ?`,
		storage.DeliveryFailed,
		"recovery",
		"service stopped before delivery finished",
		formatTime(recoveredAt),
		storage.DeliveryRunning,
	)
	if err != nil {
		return fmt.Errorf("recover running deliveries: %w", err)
	}

	return nil
}

const failureSelect = `
SELECT
    id,
    delivery_id,
    target_id,
    repository_full_name,
    event,
    stage,
    reason,
    retryable,
    finished_at
FROM deliveries`

// ListFailures returns one filtered page of sanitized delivery failures.
func (s *Store) ListFailures(
	ctx context.Context,
	targetID string,
	page storage.FailurePageRequest,
) (storage.FailurePage, error) {
	limit := pageLimit(page.Limit)
	clauses, arguments := failureFilters(targetID, page)
	total, err := countHistory(ctx, s.db, failureSelect, clauses, arguments)
	if err != nil {
		return storage.FailurePage{}, fmt.Errorf("count delivery failures: %w", err)
	}
	clauses, arguments = addHistoryCursor("id", clauses, arguments, page.HistoryPageRequest)
	direction, err := historyDirection(page.Order)
	if err != nil {
		return storage.FailurePage{}, err
	}
	arguments = append(arguments, limit+1)
	// #nosec G202 -- clauses and direction come only from fixed internal constants;
	// every request value remains a bound parameter.
	query := failureSelect + " WHERE " + strings.Join(clauses, " AND ") +
		" ORDER BY id " + direction + " LIMIT ?"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return storage.FailurePage{}, fmt.Errorf("list delivery failures: %w", err)
	}

	items, err := collectRows(rows, scanDeliveryFailure)
	if err != nil {
		return storage.FailurePage{}, fmt.Errorf("read delivery failures: %w", err)
	}

	return failurePage(items, limit, total), nil
}

func failureFilters(
	targetID string,
	page storage.FailurePageRequest,
) ([]string, []any) {
	clauses := []string{"target_id = ?", "status = ?"}
	arguments := []any{targetID, storage.DeliveryFailed}
	if page.Query != "" {
		clauses = append(clauses, `(instr(lower(delivery_id), lower(?)) > 0
OR instr(lower(repository_full_name), lower(?)) > 0
OR instr(lower(event), lower(?)) > 0
OR instr(lower(stage), lower(?)) > 0
OR instr(lower(reason), lower(?)) > 0)`)
		for range 5 {
			arguments = append(arguments, page.Query)
		}
	}
	if page.Retryable != nil {
		clauses = append(clauses, "retryable = ?")
		arguments = append(arguments, *page.Retryable)
	}

	return clauses, arguments
}

// PruneDeliveries applies retention only to finished deliveries. Running work
// is never discarded.
func (s *Store) PruneDeliveries(ctx context.Context, finishedBefore time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
DELETE FROM deliveries
WHERE finished_at IS NOT NULL AND finished_at < ?`, formatTime(finishedBefore)); err != nil {
		return fmt.Errorf("prune deliveries: %w", err)
	}

	return nil
}

func (s *Store) checkDeliveryUpdate(
	ctx context.Context,
	result sql.Result,
	claimID int64,
) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read delivery update result: %w", err)
	}

	if changed != 0 {
		return nil
	}

	var exists int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM deliveries WHERE id = ?`, claimID).Scan(&exists); err != nil {
		return fmt.Errorf("classify delivery update: %w", err)
	}

	if exists == 0 {
		return storage.ErrNotFound
	}

	return storage.ErrConflict
}

func scanDeliveryFailure(scanner rowScanner) (storage.DeliveryFailure, error) {
	var failure storage.DeliveryFailure
	var occurredAt string

	if err := scanner.Scan(
		&failure.ID,
		&failure.DeliveryID,
		&failure.TargetID,
		&failure.RepositoryFullName,
		&failure.Event,
		&failure.Stage,
		&failure.Reason,
		&failure.Retryable,
		&occurredAt,
	); err != nil {
		return storage.DeliveryFailure{}, err
	}

	parsed, err := parseTime(occurredAt)
	if err != nil {
		return storage.DeliveryFailure{}, err
	}

	failure.OccurredAt = parsed

	return failure, nil
}

func failurePage(items []storage.DeliveryFailure, limit, total int) storage.FailurePage {
	page := storage.FailurePage{Items: items, Total: total}
	if len(items) <= limit {
		return page
	}

	page.Items = items[:limit]
	page.NextCursor = page.Items[len(page.Items)-1].ID

	return page
}
