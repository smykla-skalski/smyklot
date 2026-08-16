package sqlstore

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
)

type pendingCIQuietDeadline struct {
	id             int64
	revision       int64
	lastProgressAt StoredTime
}

// RetuneQuietPeriod rewrites durable passing deadlines after the runtime
// quiet period changes. Actively leased rows are left to their worker, which
// reads the new timing before its optimistic transition; touching them here
// could invalidate a claim after its external merge has started.
func (s *Store) RetuneQuietPeriod(
	ctx context.Context,
	request pendingci.RetuneQuietPeriodRequest,
) (int64, error) {
	if err := request.Validate(); err != nil {
		return 0, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin pending CI quiet-period retune: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
SELECT id, revision, last_progress_at
FROM pending_ci_requests
WHERE lifecycle = ? AND next_check_trigger = ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
ORDER BY id`,
		pendingci.LifecycleArmed,
		pendingci.TriggerQuietPeriod,
		request.ChangedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("read pending CI quiet-period deadlines: %w", err)
	}

	deadlines := make([]pendingCIQuietDeadline, 0)
	for rows.Next() {
		var deadline pendingCIQuietDeadline
		if err := rows.Scan(&deadline.id, &deadline.revision, &deadline.lastProgressAt); err != nil {
			_ = rows.Close()

			return 0, fmt.Errorf("scan pending CI quiet-period deadline: %w", err)
		}
		deadlines = append(deadlines, deadline)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()

		return 0, fmt.Errorf("iterate pending CI quiet-period deadlines: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close pending CI quiet-period deadlines: %w", err)
	}

	var changed int64
	for _, deadline := range deadlines {
		result, err := tx.ExecContext(ctx, `
UPDATE pending_ci_requests SET
    next_check_at = ?, lease_expires_at = NULL, updated_at = ?, revision = revision + 1
WHERE id = ? AND lifecycle = ? AND next_check_trigger = ? AND revision = ?
  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`,
			deadline.lastProgressAt.Time().Add(request.PassingQuiet),
			request.ChangedAt,
			deadline.id,
			pendingci.LifecycleArmed,
			pendingci.TriggerQuietPeriod,
			deadline.revision,
			request.ChangedAt,
		)
		if err != nil {
			return 0, fmt.Errorf("retune pending CI quiet-period deadline: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("read pending CI quiet-period retune result: %w", err)
		}
		changed += affected
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit pending CI quiet-period retune: %w", err)
	}

	return changed, nil
}
