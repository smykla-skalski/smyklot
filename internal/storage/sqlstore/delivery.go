package sqlstore

import (
	"context"
	"database/sql"
	"errors"
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

	// A retained claim conflicts and returns no row, which is how a duplicate
	// delivery is recognized without asking the driver how many rows changed.
	var claimID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO deliveries (
    claim_key, delivery_id, target_id, repository_id, repository_full_name,
    event, status, payload, claimed_at, next_attempt_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING
RETURNING id`,
		claim.ClaimKey,
		claim.DeliveryID,
		claim.TargetID,
		claim.RepositoryID,
		claim.RepositoryFullName,
		claim.Event,
		storage.DeliveryRunning,
		claim.Payload,
		claim.ClaimedAt,
		claim.ClaimedAt,
	).Scan(&claimID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return storage.DeliveryClaimResult{}, fmt.Errorf("claim delivery: %w", err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		var status storage.DeliveryStatus
		if err := tx.QueryRowContext(ctx, `
	SELECT status FROM deliveries
	WHERE claim_key = ?
	  AND (status IN (?, ?) OR (status = ? AND retryable = FALSE))`,
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
	if err := tx.Commit(); err != nil {
		return storage.DeliveryClaimResult{}, fmt.Errorf("commit delivery claim: %w", err)
	}

	return storage.DeliveryClaimResult{
		ID:          claimID,
		Disposition: storage.DeliveryClaimAccepted,
	}, nil
}

// LeaseDelivery atomically reserves the oldest ready durable payload for one
// executor. When nothing is ready it reports the earliest retry or lease expiry
// so the dispatcher can sleep without polling.
func (s *Store) LeaseDelivery(
	ctx context.Context,
	now time.Time,
	leaseExpiresAt time.Time,
) (storage.DeliveryLeaseResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.DeliveryLeaseResult{}, fmt.Errorf("begin delivery lease: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	work, err := selectReadyDelivery(ctx, tx, now)
	if errors.Is(err, sql.ErrNoRows) {
		availableAt, availableErr := nextDeliveryAvailability(ctx, tx)
		if availableErr != nil {
			return storage.DeliveryLeaseResult{}, availableErr
		}
		if err := tx.Commit(); err != nil {
			return storage.DeliveryLeaseResult{}, fmt.Errorf("commit empty delivery lease: %w", err)
		}

		return storage.DeliveryLeaseResult{AvailableAt: availableAt}, nil
	}
	if err != nil {
		return storage.DeliveryLeaseResult{}, err
	}

	result, err := tx.ExecContext(ctx, `
UPDATE deliveries SET lease_expires_at = ?, attempt_count = attempt_count + 1
WHERE id = ? AND status = ? AND payload IS NOT NULL
  AND next_attempt_at <= ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`,
		leaseExpiresAt,
		work.ID,
		storage.DeliveryRunning,
		now,
		now,
	)
	if err != nil {
		return storage.DeliveryLeaseResult{}, fmt.Errorf("lease delivery: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return storage.DeliveryLeaseResult{}, fmt.Errorf("read delivery lease result: %w", err)
	}
	if changed != 1 {
		return storage.DeliveryLeaseResult{}, storage.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return storage.DeliveryLeaseResult{}, fmt.Errorf("commit delivery lease: %w", err)
	}
	work.Attempt++

	return storage.DeliveryLeaseResult{Work: &work}, nil
}

func selectReadyDelivery(
	ctx context.Context,
	tx *transaction,
	now time.Time,
) (storage.DeliveryWork, error) {
	var work storage.DeliveryWork
	var repositoryID sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT id, claim_key, delivery_id, target_id, repository_id,
       repository_full_name, event, payload, attempt_count
FROM deliveries
WHERE status = ? AND payload IS NOT NULL
  AND next_attempt_at <= ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
ORDER BY next_attempt_at, id
LIMIT 1`,
		storage.DeliveryRunning,
		now,
		now,
	).Scan(
		&work.ID,
		&work.ClaimKey,
		&work.DeliveryID,
		&work.TargetID,
		&repositoryID,
		&work.RepositoryFullName,
		&work.Event,
		&work.Payload,
		&work.Attempt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.DeliveryWork{}, sql.ErrNoRows
		}

		return storage.DeliveryWork{}, fmt.Errorf("select ready delivery: %w", err)
	}
	if repositoryID.Valid {
		work.RepositoryID = &repositoryID.String
	}

	return work, nil
}

func nextDeliveryAvailability(ctx context.Context, tx *transaction) (*time.Time, error) {
	var available StoredTime
	err := tx.QueryRowContext(ctx, `
SELECT MIN(
    CASE
        WHEN lease_expires_at IS NOT NULL AND lease_expires_at > next_attempt_at
            THEN lease_expires_at
        ELSE next_attempt_at
    END
)
FROM deliveries
WHERE status = ? AND payload IS NOT NULL`, storage.DeliveryRunning).Scan(&available)
	if err != nil {
		return nil, fmt.Errorf("read next delivery availability: %w", err)
	}
	if !available.Valid() {
		return nil, nil
	}
	parsed := available.Time()

	return &parsed, nil
}

// RetryDelivery clears an executor lease and schedules a transiently failed
// payload for another attempt.
func (s *Store) RetryDelivery(ctx context.Context, change storage.DeliveryRetryChange) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE deliveries SET
    stage = ?,
    reason = ?,
    retryable = TRUE,
    next_attempt_at = ?,
    lease_expires_at = NULL
WHERE id = ? AND status = ?`,
		change.Stage,
		change.Reason,
		change.RetryAt,
		change.ClaimID,
		storage.DeliveryRunning,
	)
	if err != nil {
		return fmt.Errorf("retry delivery: %w", err)
	}

	return s.checkDeliveryUpdate(ctx, result, change.ClaimID)
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
		completedAt,
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
		change.FailedAt,
		change.ClaimID,
		storage.DeliveryRunning,
		storage.DeliveryFailed,
	)
	if err != nil {
		return fmt.Errorf("fail delivery: %w", err)
	}

	return s.checkDeliveryUpdate(ctx, result, change.ClaimID)
}

// RecoverRunningDeliveries requeues durable payloads that belonged to the
// previous process and retains the old failure behavior for pre-inbox rows.
// The deployment is intentionally single-replica, so no running row can still
// have an executor when a new store owner starts.
func (s *Store) RecoverRunningDeliveries(ctx context.Context, recoveredAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin running delivery recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
UPDATE deliveries SET
    lease_expires_at = NULL,
    next_attempt_at = ?,
    stage = NULL,
    reason = NULL,
    retryable = NULL
WHERE status = ? AND payload IS NOT NULL`,
		recoveredAt,
		storage.DeliveryRunning,
	); err != nil {
		return fmt.Errorf("requeue durable deliveries: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE deliveries SET
    status = ?,
    stage = ?,
    reason = ?,
    retryable = TRUE,
    finished_at = ?
WHERE status = ? AND payload IS NULL`,
		storage.DeliveryFailed,
		"recovery",
		"service stopped before delivery finished",
		recoveredAt,
		storage.DeliveryRunning,
	); err != nil {
		return fmt.Errorf("recover running deliveries: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit running delivery recovery: %w", err)
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
	order, err := failurePageOrder(page.Order)
	if err != nil {
		return storage.FailurePage{}, err
	}
	offset := max(page.Offset, 0)
	arguments = append(arguments, limit+1, offset)
	// #nosec G202 -- clauses and direction come only from fixed internal constants;
	// every request value remains a bound parameter.
	query := failureSelect + " WHERE " + strings.Join(clauses, " AND ") +
		" ORDER BY " + order + " LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return storage.FailurePage{}, fmt.Errorf("list delivery failures: %w", err)
	}

	items, err := collectRows(rows, scanDeliveryFailure)
	if err != nil {
		return storage.FailurePage{}, fmt.Errorf("read delivery failures: %w", err)
	}

	return failurePage(items, limit, total, offset), nil
}

func failurePageOrder(order storage.HistoryOrder) (string, error) {
	switch order {
	case "", storage.HistoryNewest:
		return "id DESC", nil
	case storage.HistoryOldest:
		return "id ASC", nil
	case storage.HistoryStatusAscending:
		return "retryable ASC, id DESC", nil
	case storage.HistoryStatusDescending:
		return "retryable DESC, id DESC", nil
	case storage.HistoryRepositoryAscending:
		return caseFold("repository_full_name") + " ASC, id DESC", nil
	case storage.HistoryRepositoryDescending:
		return caseFold("repository_full_name") + " DESC, id DESC", nil
	default:
		return "", fmt.Errorf("unsupported failure order %q", order)
	}
}

func failureFilters(
	targetID string,
	page storage.FailurePageRequest,
) ([]string, []any) {
	clauses := []string{"target_id = ?", "status = ?"}
	arguments := []any{targetID, storage.DeliveryFailed}
	if page.Query != "" {
		columns := []string{"delivery_id", "repository_full_name", "event", "stage", "reason"}
		clauses = append(clauses, containsAnyClause(columns...))
		arguments = append(arguments, containsArguments(page.Query, len(columns))...)
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
WHERE finished_at IS NOT NULL AND finished_at < ?`, finishedBefore); err != nil {
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
	var occurredAt StoredTime

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

	failure.OccurredAt = occurredAt.Time()

	return failure, nil
}

func failurePage(items []storage.DeliveryFailure, limit, total, offset int) storage.FailurePage {
	page := storage.FailurePage{Items: items, Total: total}
	if len(items) <= limit {
		return page
	}

	page.Items = items[:limit]
	page.NextOffset = offset + limit

	return page
}
