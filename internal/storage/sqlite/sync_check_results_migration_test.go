package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

func TestSyncCheckResultsPreserveLegacyEvidenceAfterPruning(t *testing.T) {
	ctx := context.Background()
	db := openLegacyDatabase(t, ctx, filepath.Join(t.TempDir(), "check-results.db"), 65)
	dialect := Dialect{}
	stamp := "2026-09-14T01:00:00.000000000Z"
	details := `{"outcome":{"completed_at":"2026-09-14T01:00:00Z","disposition":"checked","summary":"Original comparison","counts":{"matched":1}},"result_plan_id":"original-plan","future_field":"preserve"}`
	evidence := `{"repository_id":"repo","repository":"owner/original-name","outcome":"matched","observed_at":"2026-09-13T23:00:00Z"}`
	for _, row := range []struct{ id, kind, details string }{
		{"completed-check", "sync_scan", details},
		{"plan-only", "sync_scan", `{"result_plan_id":"legacy-plan"}`},
		{"unknown-check", "sync_scan", `{}`},
		{"other-work", "catalog_refresh", details},
	} {
		_, err := db.ExecContext(ctx, dialect.Rebind(`INSERT INTO queue_items (id, kind, lane, title, state, priority, window_mode, not_before, eligible_at, created_at, updated_at, target_id, details) VALUES (?, ?, 'maintenance', 'Historical work', 'succeeded', 'normal', 'respect', ?, ?, ?, ?, 'target', ?)`), row.id, row.kind, stamp, stamp, stamp, stamp, row.details)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, dialect.Rebind(`INSERT INTO sync_check_observations (queue_id, ordinal, evidence) VALUES ('completed-check', 1, ?)`), evidence); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := sqlstore.Migrate(ctx, db, dialect, migrations); err != nil {
			t.Fatal(err)
		}
		assertRetainedLegacyCheck(t, db, details, evidence)
		// The second migration/read happens after worker cleanup.
		if _, err := db.ExecContext(ctx, `DELETE FROM queue_items`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM sync_check_results WHERE check_id = 'completed-check'`); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_check_observations`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("domain removal left orphaned evidence")
	}
}

func assertRetainedLegacyCheck(t *testing.T, db *sql.DB, details, evidence string) {
	t.Helper()
	ctx := t.Context()
	var stored, target, observation string
	if err := db.QueryRowContext(ctx, `SELECT target_id, details FROM sync_check_results WHERE check_id = 'completed-check'`).Scan(&target, &stored); err != nil {
		t.Fatal(err)
	}
	if target != "target" || stored != details {
		t.Fatalf("changed comparison: %q %q", target, stored)
	}
	if err := db.QueryRowContext(ctx, `SELECT evidence FROM sync_check_observations WHERE queue_id = 'completed-check' AND ordinal = 1`).Scan(&observation); err != nil {
		t.Fatal(err)
	}
	if observation != evidence {
		t.Fatalf("changed evidence: %q", observation)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_check_results`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("invented or lost results: %d", count)
	}
	if err := db.QueryRowContext(ctx, `SELECT details FROM sync_check_results WHERE check_id = 'plan-only'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != `{"result_plan_id":"legacy-plan"}` {
		t.Fatalf("invented legacy outcome: %s", stored)
	}
}
