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

func lockPendingCIPolicy(ctx context.Context, tx runner, dialect Dialect) error {
	var singleton int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT singleton FROM pending_ci_policy_lock WHERE singleton = 1"+dialect.RowLock(),
	).Scan(&singleton); err != nil {
		return fmt.Errorf("lock pending CI policy: %w", err)
	}

	return nil
}

// RetuneQuietPeriod rewrites durable passing deadlines after the runtime
// quiet period changes. A leased row that has not claimed its merge is safe to
// invalidate: either its optimistic write wins first and this retune follows,
// or this revision bump makes that write retry with current policy. A claimed
// merge is the intentional cutoff because its external effects have begun.
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
	if err := lockPendingCIPolicy(ctx, tx, s.dialect); err != nil {
		return 0, err
	}

	changed, err := retuneQuietPeriod(ctx, tx, s.dialect, request)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit pending CI quiet-period retune: %w", err)
	}

	return changed, nil
}

func retuneQuietPeriod(
	ctx context.Context,
	tx runner,
	dialect Dialect,
	request pendingci.RetuneQuietPeriodRequest,
) (int64, error) {
	query := `
SELECT p.id, p.revision, p.last_progress_at
FROM pending_ci_requests p
WHERE p.lifecycle = ? AND p.next_check_trigger = ? AND p.merge_phase = ?`
	arguments := []any{
		pendingci.LifecycleArmed,
		pendingci.TriggerQuietPeriod,
		pendingci.MergeWaiting,
	}
	if request.TargetID != "" {
		query += " AND p.target_id = ?"
		arguments = append(arguments, request.TargetID)
	}
	if request.RepositoryID != "" {
		query += " AND p.repository_id = ?"
		arguments = append(arguments, request.RepositoryID)
	}
	if request.InheritedOnly {
		query += ` AND EXISTS (
SELECT 1 FROM repositories r`
		if request.TargetID == "" {
			query += " JOIN targets t ON t.id = r.target_id"
		}
		query += `
WHERE r.id = p.repository_id AND r.target_id = p.target_id
  AND r.pending_ci_quiet_period_seconds_override IS NULL`
		if request.TargetID == "" {
			query += " AND t.pending_ci_quiet_period_seconds_override IS NULL"
		}
		query += ")"
	}
	query += " ORDER BY p.id" + dialect.RowLock()
	rows, err := tx.QueryContext(ctx, query, arguments...)
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
	AND merge_phase = ?`,
			deadline.lastProgressAt.Time().Add(request.PassingQuiet),
			request.ChangedAt,
			deadline.id,
			pendingci.LifecycleArmed,
			pendingci.TriggerQuietPeriod,
			deadline.revision,
			pendingci.MergeWaiting,
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

	return changed, nil
}
