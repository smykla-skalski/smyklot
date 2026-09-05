package sqlstore

import (
	"context"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// A finished run leaves the live slot. Persist its unresolved problems beside
// repository state so the panel cannot turn a failed run into "up to date".
// The empty digest also makes the next reconciliation retry that kind.
func recordSyncFailures(ctx context.Context, tx *transaction, planID string, now time.Time, stopped bool) error {
	statesClause := "('failed', 'skipped')"
	if stopped {
		// A run that stopped before producing action outcomes still needs recovery.
		// Keep its execution error in the Root queue; pending actions get the public
		// fallback below, never the potentially sensitive infrastructure detail.
		statesClause = "('failed', 'skipped', 'pending')"
	}
	rows, err := tx.QueryContext(ctx, `
SELECT repository_id, kind, error FROM sync_plan_actions
WHERE plan_id = ? AND state IN `+statesClause+` ORDER BY id`, planID)
	if err != nil {
		return fmt.Errorf("read unresolved sync actions: %w", err)
	}
	states, err := collectRows(rows, func(row rowScanner) (orgsync.RepositoryState, error) {
		state := orgsync.RepositoryState{AppliedAt: now}
		err := row.Scan(&state.RepositoryID, &state.Kind, &state.Problem)
		return state, err
	})
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, state := range states {
		key := state.RepositoryID + "/" + string(state.Kind)
		if seen[key] {
			continue
		}
		seen[key] = true
		if state.Problem == "" {
			state.Problem = "This change could not be applied; check the repository and retry sync"
		}
		if err := upsertSyncRepositoryState(ctx, tx, state); err != nil {
			return err
		}
	}
	return nil
}
