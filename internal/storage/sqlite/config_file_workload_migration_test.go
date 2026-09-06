package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func TestConfigurationWorkloadMigrationPreservesExistingWork(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "configuration-work.db")
	legacy := openLegacyDatabase(t, ctx, path, 50)
	now := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)
	seedSQLiteQueueSources(t, ctx, legacy, now)
	shared := sqlstore.New(legacy, Dialect{})
	target := "installation:queue"
	_, err := shared.EnsureRecurringWork(ctx, workqueue.RecurringClaim{
		Kind: workqueue.KindConfigMigration, TargetID: &target,
		Title: "Existing scheduled work", Now: now, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Keep a non-pristine policy and a pruned event beyond the remaining IDs.
	for _, query := range []string{
		`UPDATE queue_policies SET cadence_seconds = 173, revision = 8 WHERE kind = 'config_migration'`,
		`INSERT INTO queue_events (id, queue_item_id, kind, state, summary, created_at)
SELECT 900, id, 'test', state, 'Pruned event', updated_at FROM queue_items LIMIT 1`,
		`DELETE FROM queue_events WHERE id = 900`,
	} {
		if _, err := legacy.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	queries := []string{
		`SELECT * FROM queue_items ORDER BY id`,
		`SELECT * FROM queue_events ORDER BY id`,
		`SELECT * FROM queue_policies WHERE kind <> 'config_file_sync' ORDER BY kind, scope_id`,
		`SELECT name, sql FROM sqlite_master WHERE type = 'index' AND name LIKE 'queue_%' ORDER BY name`,
	}
	before := make([][][]any, len(queries))
	for index, query := range queries {
		before[index] = configurationMigrationRows(t, ctx, legacy, query)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for index, query := range queries {
		if got := configurationMigrationRows(t, ctx, store.DB(), query); !reflect.DeepEqual(before[index], got) {
			t.Fatalf("migration changed existing rows or indexes for %s", query)
		}
	}
	if rows := configurationMigrationRows(t, ctx, store.DB(), `PRAGMA foreign_key_check`); len(rows) != 0 {
		t.Fatalf("migration broke foreign keys: %v", rows)
	}
	if _, err := store.DB().ExecContext(ctx, `INSERT INTO queue_events (queue_item_id, kind, state, summary, created_at)
SELECT id, 'test', state, 'Next event', updated_at FROM queue_items LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	var next int64
	if err := store.DB().QueryRowContext(ctx, `SELECT MAX(id) FROM queue_events`).Scan(&next); err != nil || next != 901 {
		t.Fatalf("event cursor regressed after migration: %d (%v)", next, err)
	}
}

func configurationMigrationRows(t *testing.T, ctx context.Context, db *sql.DB, query string) [][]any {
	t.Helper()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result [][]any
	for rows.Next() {
		values, pointers := make([]any, len(columns)), make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
