package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// TestSettingsCheckpointMigrationPreservesLegacySyncHistory is the production
// engine counterpart to SQLite's compatibility proof.
func TestSettingsCheckpointMigrationPreservesLegacySyncHistory(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SMYKLOT_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("SMYKLOT_TEST_POSTGRES_DSN is not set, so there is no server to migrate")
	}

	ctx := context.Background()
	scoped := scopedSchema(t, ctx, dsn)
	legacy, err := sqlstore.MigrationsBefore(migrations, 24)
	if err != nil {
		t.Fatalf("cut the migration series: %v", err)
	}
	db := openPool(t, ctx, scoped)
	if err := sqlstore.Migrate(ctx, db, Dialect{}, legacy); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}
	now := time.Date(2026, time.August, 23, 8, 0, 0, 0, time.UTC)

	statements := []struct {
		query     string
		arguments []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ('github:owner', 'github', 'owner', 'owner', 'Owner', $1)`, []any{now}},
		{`INSERT INTO targets (
id, installation_id, kind, account_id, settings_updated_at, synced_at
) VALUES ('installation:one', '1', 'Organization', 'github:owner', $1, $2)`, []any{now, now}},
		{`INSERT INTO sync_config_checkpoints (
target_id, actor_account_id, action, created_at
) VALUES ('installation:one', 'github:owner', 'save', $1) RETURNING id`, []any{now}},
	}
	for index, statement := range statements[:2] {
		if _, err := db.ExecContext(ctx, statement.query, statement.arguments...); err != nil {
			t.Fatalf("seed legacy settings history %d: %v", index, err)
		}
	}
	var checkpointID int64
	if err := db.QueryRowContext(ctx, statements[2].query, now).Scan(&checkpointID); err != nil {
		t.Fatalf("seed legacy sync checkpoint: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sync_config_checkpoint_items (
checkpoint_id, kind, enabled, document, digest, revision
) VALUES ($1, 'labels', true, '{"labels":[]}', 'legacy-digest', 7)`, checkpointID); err != nil {
		t.Fatalf("seed legacy sync checkpoint item: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := Open(ctx, scoped)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var revision, genericHeaders, genericItems int64
	if err := store.DB().QueryRowContext(ctx, `SELECT
    (SELECT revision FROM sync_config_checkpoint_items WHERE checkpoint_id = $1),
    (SELECT COUNT(*) FROM settings_checkpoints),
    (SELECT COUNT(*) FROM settings_checkpoint_items)`, checkpointID).
		Scan(&revision, &genericHeaders, &genericItems); err != nil {
		t.Fatalf("read migrated settings history: %v", err)
	}
	if revision != 7 || genericHeaders != 1 || genericItems != 1 {
		t.Fatalf("migration changed history: revision=%d headers=%d items=%d",
			revision, genericHeaders, genericItems)
	}

	var genericCheckpointID int64
	if err := store.DB().QueryRowContext(ctx, `INSERT INTO settings_checkpoints (
scope, target_id, actor_account_id, action, created_at
) VALUES ('installation', 'installation:one', 'github:owner', 'save', $1)
RETURNING id`, now).Scan(&genericCheckpointID); err != nil {
		t.Fatalf("insert generic settings checkpoint: %v", err)
	}
	insertAudit := `INSERT INTO audit_entries (
target_id, settings_checkpoint_id, actor_account_id, action, summary, created_at
) VALUES ('installation:one', $1, 'github:owner', 'settings.saved', 'Saved settings', $2)`
	if _, err := store.DB().ExecContext(ctx, insertAudit, genericCheckpointID, now); err != nil {
		t.Fatalf("link generic settings checkpoint audit: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, insertAudit, genericCheckpointID, now); err == nil {
		t.Fatal("linked one generic settings checkpoint to two target audit rows")
	}
	var linked int64
	if err := store.DB().QueryRowContext(ctx, `
SELECT settings_checkpoint_id FROM audit_entries WHERE action = 'settings.saved'`).
		Scan(&linked); err != nil {
		t.Fatalf("read generic settings audit link: %v", err)
	}
	if linked != genericCheckpointID {
		t.Fatalf("generic settings audit link = %d, want %d", linked, genericCheckpointID)
	}
}
