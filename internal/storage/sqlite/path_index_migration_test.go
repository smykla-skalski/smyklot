package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// TestPathIndexMigrationDropsLegacyRows migrates a database that already has
// path lists in it.
//
// The conformance suite builds every database from nothing, so it proves the
// columns exist and says nothing about a deployment that already holds rows -
// which is every deployment. These rows were written as one string with a
// newline between every path, and they are dropped rather than converted: the
// table is a cache of what GitHub answered, so the next sweep reads the list
// again and writes it as JSON. The commit goes with them - left behind, a
// repository would look settled at a commit whose file list is no longer
// stored, and the finder would answer with nothing for ever.
func TestPathIndexMigrationDropsLegacyRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "path-index.db")
	db := openLegacyDatabase(t, ctx, path, 33)
	now := time.Date(2026, time.August, 19, 9, 0, 0, 0, time.UTC)

	stamp := now.Format("2006-01-02T15:04:05.000000000Z")
	for _, seed := range []struct {
		query     string
		arguments []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
          VALUES ('github:1', 'github', '1', 'smykla-skalski', 'Smykla', ?)`, []any{stamp}},
		{
			`INSERT INTO targets (
              id, installation_id, kind, account_id, settings_updated_at, synced_at
          ) VALUES ('installation:77', '77', 'Organization', 'github:1', ?, ?)`,
			[]any{stamp, stamp},
		},
		{
			`INSERT INTO repositories (
              id, target_id, name, full_name, private, settings_updated_at, synced_at
          ) VALUES ('9001', 'installation:77', 'smyklot', 'smykla-skalski/smyklot', 0, ?, ?)`,
			[]any{stamp, stamp},
		},
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
