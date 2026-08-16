package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	storagepostgres "github.com/smykla-skalski/smyklot/internal/storage/postgres"
	storagesqlite "github.com/smykla-skalski/smyklot/internal/storage/sqlite"
)

// TestSchemaParity asserts the two engines end up with the same shape.
//
// They get there differently on purpose: each engine replays its own migration
// history. Nothing keeps those two histories in step except this test, so
// without it a column added to one series would quietly be missing from the
// other until a query failed in production.
//
// Types are deliberately not compared. SQLite spells a boolean, a timestamp
// and a JSON document all as TEXT or INTEGER because it has nothing better,
// and the point of the second engine is to use what it does have.
func TestSchemaParity(t *testing.T) {
	t.Parallel()

	dsn := strings.TrimSpace(os.Getenv(dsnVariable))
	if dsn == "" {
		t.Skip(dsnVariable + " is not set, so there is no server to compare against")
	}

	ctx := context.Background()

	sqliteStore, err := storagesqlite.Open(ctx, filepath.Join(t.TempDir(), "parity.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqliteStore.Close() })

	// Named after the process, not fixed. `ginkgo -p` runs the suite binary once
	// per process, and a plain test in a Ginkgo package runs in every one of
	// them, so a fixed name had thirteen copies dropping and creating the same
	// schema underneath each other.
	schema := fmt.Sprintf("smyklot_parity_%d", os.Getpid())
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if _, err := admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
		t.Fatalf("reset parity schema: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create parity schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.WithoutCancel(ctx), "DROP SCHEMA "+schema+" CASCADE")
	})

	postgresStore, err := storagepostgres.Open(ctx, dsnForSchema(schema))
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	t.Cleanup(func() { _ = postgresStore.Close() })

	requireSame(t, "columns", sqliteColumns(t, ctx, sqliteStore), postgresColumns(t, ctx, admin, schema))
	requireCovered(t, "indexes", sqliteIndexes(t, ctx, sqliteStore), postgresIndexes(t, ctx, admin, schema))
}

// requireSame fails on any difference in either direction.
func requireSame(t *testing.T, what string, sqliteSet, postgresSet map[string]bool) {
	t.Helper()

	report(t, what, "sqlite but not postgres", difference(sqliteSet, postgresSet))
	report(t, what, "postgres but not sqlite", difference(postgresSet, sqliteSet))
}

// requireCovered fails only on something the shared queries rely on going
// missing. An engine is free to add its own - the GIN index over the config
// patch is exactly the kind of thing the second engine exists to allow - so an
// extra on the PostgreSQL side is reported and not treated as drift.
func requireCovered(t *testing.T, what string, sqliteSet, postgresSet map[string]bool) {
	t.Helper()

	report(t, what, "sqlite but not postgres", difference(sqliteSet, postgresSet))
	if extra := difference(postgresSet, sqliteSet); len(extra) > 0 {
		t.Logf("%s only in postgres (allowed):\n  %s", what, strings.Join(extra, "\n  "))
	}
}

func report(t *testing.T, what, direction string, entries []string) {
	t.Helper()

	if len(entries) > 0 {
		t.Errorf("%s in %s:\n  %s", what, direction, strings.Join(entries, "\n  "))
	}
}

func difference(from, to map[string]bool) []string {
	var only []string
	for key := range from {
		if !to[key] {
			only = append(only, key)
		}
	}
	sort.Strings(only)

	return only
}

// sqliteColumns describes every column as "table.column nullable=..." so the
// comparison reports a difference in terms a reader can act on.
func sqliteColumns(t *testing.T, ctx context.Context, store *storagesqlite.Store) map[string]bool {
	t.Helper()

	columns := map[string]bool{}
	for _, table := range sqliteTables(t, ctx, store) {
		rows, err := store.DB().QueryContext(ctx, "PRAGMA table_info("+table+")")
		if err != nil {
			t.Fatalf("read sqlite columns for %s: %v", table, err)
		}
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, kind string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
				t.Fatalf("scan sqlite column: %v", err)
			}
			// A primary key is never nullable, but table_info reports its
			// notnull flag as 0, so the key column has to be read as well.
			columns[describeColumn(table, name, notNull == 0 && primaryKey == 0)] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate sqlite columns: %v", err)
		}
		_ = rows.Close()
	}

	return columns
}

func sqliteTables(t *testing.T, ctx context.Context, store *storagesqlite.Store) []string {
	t.Helper()

	rows, err := store.DB().QueryContext(ctx, `
SELECT name FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
	if err != nil {
		t.Fatalf("read sqlite tables: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan sqlite table: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sqlite tables: %v", err)
	}

	return tables
}

func postgresColumns(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	schema string,
) map[string]bool {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
SELECT table_name, column_name, is_nullable
FROM information_schema.columns
WHERE table_schema = $1
ORDER BY table_name, column_name`, schema)
	if err != nil {
		t.Fatalf("read postgres columns: %v", err)
	}
	defer func() { _ = rows.Close() }()

	columns := map[string]bool{}
	for rows.Next() {
		var table, name, nullable string
		if err := rows.Scan(&table, &name, &nullable); err != nil {
			t.Fatalf("scan postgres column: %v", err)
		}
		columns[describeColumn(table, name, nullable == "YES")] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate postgres columns: %v", err)
	}

	return columns
}

func sqliteIndexes(t *testing.T, ctx context.Context, store *storagesqlite.Store) map[string]bool {
	t.Helper()

	rows, err := store.DB().QueryContext(ctx, `
SELECT name FROM sqlite_master
WHERE type = 'index' AND sql IS NOT NULL
ORDER BY name`)
	if err != nil {
		t.Fatalf("read sqlite indexes: %v", err)
	}
	defer func() { _ = rows.Close() }()

	return collectNames(t, rows)
}

func postgresIndexes(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	schema string,
) map[string]bool {
	t.Helper()

	// A unique constraint and a primary key each create an index the other
	// engine declares inline, so only explicitly created indexes are compared.
	rows, err := db.QueryContext(ctx, `
SELECT indexname FROM pg_indexes
WHERE schemaname = $1 AND indexname LIKE '%_idx'
ORDER BY indexname`, schema)
	if err != nil {
		t.Fatalf("read postgres indexes: %v", err)
	}
	defer func() { _ = rows.Close() }()

	return collectNames(t, rows)
}

func collectNames(t *testing.T, rows *sql.Rows) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan name: %v", err)
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate names: %v", err)
	}

	return names
}

func describeColumn(table, column string, nullable bool) string {
	return fmt.Sprintf("%s.%s nullable=%t", table, column, nullable)
}
