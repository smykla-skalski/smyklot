package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestPathIndexMigrationKeepsExistingRows migrates a database that already has
// path lists in it.
//
// The conformance suite builds every database from nothing, so it proves the
// columns exist and says nothing about a deployment that already holds rows -
// which is every deployment. A path list written before the commit and the
// truncation flag were recorded has to come back readable, with the commit
// empty rather than absent, because empty is what makes the next sweep read
// that repository once more instead of believing a list it cannot date.
func TestPathIndexMigrationKeepsExistingRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "path-index.db")
	db := openLegacyDatabase(t, ctx, path, "033_")
	now := time.Date(2026, time.August, 19, 9, 0, 0, 0, time.UTC)

	stamp := now.Format("2006-01-02T15:04:05.000000000Z")
	for _, seed := range []struct {
		query     string
		arguments []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
          VALUES ('github:1', 'github', '1', 'smykla-skalski', 'Smykla', ?)`, []any{stamp}},
		{`INSERT INTO targets (
              id, installation_id, kind, account_id, settings_updated_at, synced_at
          ) VALUES ('installation:77', '77', 'Organization', 'github:1', ?, ?)`,
			[]any{stamp, stamp}},
		{`INSERT INTO repositories (
              id, target_id, name, full_name, private, settings_updated_at, synced_at
          ) VALUES ('9001', 'installation:77', 'smyklot', 'smykla-skalski/smyklot', 0, ?, ?)`,
			[]any{stamp, stamp}},
	} {
		if _, err := db.ExecContext(ctx, seed.query, seed.arguments...); err != nil {
			t.Fatalf("seed catalog: %v", err)
		}
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO sync_repository_paths (repository_id, target_id, paths, observed_at)
VALUES (?, ?, ?, ?)`,
		"9001", "installation:77", "README.md\ndocs/guide.md", stamp,
	); err != nil {
		t.Fatalf("seed path list: %v", err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rows, err := store.ListSyncRepositoryPaths(ctx, "installation:77")
	if err != nil {
		t.Fatalf("read migrated path list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want one row, got %d", len(rows))
	}
	if got := len(rows[0].Paths); got != 2 {
		t.Fatalf("want the two seeded paths, got %d", got)
	}
	if rows[0].HeadSHA != "" {
		t.Fatalf("want no commit on a row written before one was recorded, got %q", rows[0].HeadSHA)
	}
	if rows[0].Partial {
		t.Fatal("want a row written before truncation was recorded to read as complete")
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
