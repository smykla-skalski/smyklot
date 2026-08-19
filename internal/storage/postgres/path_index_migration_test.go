package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// TestPathIndexMigrationDropsLegacyRows migrates a PostgreSQL database that
// already has path lists in it.
//
// The same proof SQLite carries, on the engine production actually runs. The
// conformance suite builds every database from nothing, so it says nothing
// about a deployment that already holds rows - which is every deployment - and
// this migration is the one that empties a table. An engine that dropped the
// rows on one side and not the other would leave the finder answering from a
// list stored in a shape nothing can read, for ever, on the only deployment
// that matters.
//
// Internal to the package because it has to cut the migration series before the
// destructive step, which needs the embedded `migrations` and the `Dialect`.
func TestPathIndexMigrationDropsLegacyRows(t *testing.T) {
	t.Parallel()

	dsn := strings.TrimSpace(os.Getenv("SMYKLOT_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("SMYKLOT_TEST_POSTGRES_DSN is not set, so there is no server to migrate")
	}

	ctx := context.Background()
	now := time.Date(2026, time.August, 19, 9, 0, 0, 0, time.UTC)
	scoped := scopedSchema(t, ctx, dsn)

	seedLegacyPathList(t, ctx, scoped, now)

	// Open runs the rest of the series, including the destructive step.
	store, err := Open(ctx, scoped)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rows, err := store.ListSyncRepositoryPaths(ctx, "installation:77")
	if err != nil {
		t.Fatalf("read migrated path list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("want the legacy rows dropped, got %d", len(rows))
	}

	// And the table takes what the next sweep writes, including the path that
	// could not be stored before: git permits a newline in a filename.
	awkward := []string{"README.md", "docs/a\nb.md"}
	if err := store.SetSyncRepositoryPaths(ctx, orgsync.RepositoryPaths{
		RepositoryID: "9001", TargetID: "installation:77", Paths: awkward, ObservedAt: now,
		HeadSHA: "aaaa1111",
	}); err != nil {
		t.Fatalf("write a path list after the migration: %v", err)
	}

	rows, err = store.ListSyncRepositoryPaths(ctx, "installation:77")
	if err != nil {
		t.Fatalf("read the rewritten path list: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Paths) != 2 || rows[0].Paths[1] != awkward[1] {
		t.Fatalf("want the two paths back whole, got %#v", rows)
	}

	// And the three interval columns exist and default to inheriting, which is
	// what makes an upgraded deployment keep the process's answer.
	var target, repository, runtime *int64
	if err := store.DB().QueryRowContext(ctx, `
SELECT
    (SELECT path_index_interval_seconds_override FROM targets WHERE id = 'installation:77'),
    (SELECT path_index_interval_seconds_override FROM repositories WHERE id = '9001'),
    (SELECT path_index_interval_seconds FROM runtime_settings WHERE singleton = 1)`,
	).Scan(&target, &repository, &runtime); err != nil {
		t.Fatalf("read migrated interval columns: %v", err)
	}
	if target != nil || repository != nil || runtime != nil {
		t.Fatalf("want every interval inheriting, got target=%v repository=%v runtime=%v",
			target, repository, runtime)
	}
}

// scopedSchema is a DSN pointing at a schema of this test's own, dropped after.
//
// Named after the process rather than fixed: a plain test in a Ginkgo package
// runs in every one of `ginkgo -p`'s processes, and a fixed name had all of them
// dropping and creating the same schema underneath each other.
func scopedSchema(t *testing.T, ctx context.Context, dsn string) string {
	t.Helper()

	schema := fmt.Sprintf("smyklot_pathindex_%d", os.Getpid())

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	if _, err := admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.WithoutCancel(ctx), "DROP SCHEMA "+schema+" CASCADE")
	})

	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}

	return dsn + separator + "search_path=" + schema
}

// seedLegacyPathList brings a schema to the state every deployment upgrading
// into the encoding change is in: the series up to it, and a path list written
// the old way - one string with a newline between every path.
func seedLegacyPathList(t *testing.T, ctx context.Context, dsn string, now time.Time) {
	t.Helper()

	legacy, err := sqlstore.MigrationsBefore(migrations, 21)
	if err != nil {
		t.Fatalf("cut the migration series: %v", err)
	}

	pool := openPool(t, ctx, dsn)
	if err := sqlstore.Migrate(ctx, pool, Dialect{}, legacy); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}

	for _, seed := range []struct {
		query     string
		arguments []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
          VALUES ('github:1', 'github', '1', 'smykla-skalski', 'Smykla', $1)`, []any{now}},
		{
			`INSERT INTO targets (
              id, installation_id, kind, account_id, settings_updated_at, synced_at
          ) VALUES ('installation:77', '77', 'Organization', 'github:1', $1, $2)`,
			[]any{now, now},
		},
		{
			`INSERT INTO repositories (
              id, target_id, name, full_name, private, settings_updated_at, synced_at
          ) VALUES ('9001', 'installation:77', 'smyklot', 'smykla-skalski/smyklot', false, $1, $2)`,
			[]any{now, now},
		},
		{
			`INSERT INTO sync_repository_paths (repository_id, target_id, paths, observed_at)
          VALUES ('9001', 'installation:77', $1, $2)`,
			[]any{"README.md\ndocs/guide.md", now},
		},
	} {
		if _, err := pool.ExecContext(ctx, seed.query, seed.arguments...); err != nil {
			t.Fatalf("seed legacy rows: %v", err)
		}
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("close the legacy pool: %v", err)
	}
}

// openPool connects the way Open does, so the legacy half of this test reads
// and writes timestamps in the same zone the migrated half will.
func openPool(t *testing.T, ctx context.Context, dsn string) *sql.DB {
	t.Helper()

	settings, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse connection string: %v", err)
	}
	settings.RuntimeParams["timezone"] = sessionTimeZone

	pool := stdlib.OpenDB(*settings)
	if err := pool.PingContext(ctx); err != nil {
		t.Fatalf("reach postgres: %v", err)
	}

	return pool
}
