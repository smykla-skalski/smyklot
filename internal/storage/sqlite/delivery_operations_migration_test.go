package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

func TestDeliveryOperationsMigrationPreservesFailedRuns(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "delivery-operations.db")
	db := openLegacyDatabase(t, ctx, path, 54)
	dialect := Dialect{}
	var firstID, lastID int64
	for index, retryable := range []bool{true, false} {
		err := db.QueryRowContext(ctx, dialect.Rebind(`INSERT INTO deliveries
  (claim_key, delivery_id, target_id, repository_full_name, event, status, retryable,
   reason, claimed_at, finished_at)
  VALUES ('event-key', 'github-delivery', 'target', 'owner/repo', 'issue_comment',
   'failed', ?, 'retained failure', '2026-09-13T12:00:00.000000000Z',
   '2026-09-13T12:01:00.000000000Z') RETURNING id`), retryable).Scan(&lastID)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			firstID = lastID
		}
	}
	for range 2 {
		if err := sqlstore.Migrate(ctx, db, dialect, migrations); err != nil {
			t.Fatal(err)
		}
		var order, current, count int64
		if err := db.QueryRowContext(ctx, `SELECT source_order, current_delivery_id
  FROM delivery_operations WHERE claim_key = 'event-key'`).Scan(&order, &current); err != nil {
			t.Fatal(err)
		}
		if order != firstID || current != lastID {
			t.Fatalf("operation order=%d current=%d, want %d/%d", order, current, firstID, lastID)
		}
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deliveries
  WHERE claim_key = 'event-key' AND status = 'failed' AND reason = 'retained failure'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("migration rewrote failure history: %d rows", count)
		}
	}
}
