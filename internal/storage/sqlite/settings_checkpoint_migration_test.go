package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// TestSettingsCheckpointMigrationPreservesLegacySyncHistory proves the new
// bounded history starts empty and does not rewrite the shipped Sync history or
// its audit foreign key.
func TestSettingsCheckpointMigrationPreservesLegacySyncHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "settings-checkpoints.db")
	db := openLegacyDatabase(t, ctx, path, 39)
	now := time.Date(2026, time.August, 23, 8, 0, 0, 0, time.UTC).
		Format(time.RFC3339Nano)

	statements := []string{
		`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ('github:owner', 'github', 'owner', 'owner', 'Owner', ?)`,
		`INSERT INTO targets (
id, installation_id, kind, account_id, settings_updated_at, synced_at
) VALUES ('installation:one', '1', 'Organization', 'github:owner', ?, ?)`,
		`INSERT INTO sync_config_checkpoints (
id, target_id, actor_account_id, action, created_at
) VALUES (41, 'installation:one', 'github:owner', 'save', ?)`,
		`INSERT INTO sync_config_checkpoints (
id, target_id, actor_account_id, action, restored_from_id, created_at
) VALUES (42, 'installation:one', 'github:owner', 'restore', 41, ?)`,
		`INSERT INTO sync_config_checkpoint_items (
checkpoint_id, kind, enabled, document, digest, revision
) VALUES (41, 'labels', 1, '{"labels":[]}', 'legacy-digest', 7)`,
		`INSERT INTO audit_entries (
target_id, sync_config_checkpoint_id, actor_account_id, action, summary, created_at
) VALUES ('installation:one', 42, 'github:owner', 'sync.config.restored',
          'Restored labels sync configuration', ?)`,
	}
	arguments := [][]any{{now}, {now, now}, {now}, {now}, nil, {now}}
	for index, statement := range statements {
		if _, err := db.ExecContext(ctx, statement, arguments[index]...); err != nil {
			t.Fatalf("seed legacy settings history: %v\n%s", err, statement)
		}
	}

	if err := sqlstore.Migrate(ctx, db, Dialect{}, migrations); err != nil {
		t.Fatalf("apply settings checkpoint migration: %v", err)
	}

	assertLegacySettingsHistory(t, ctx, db)
	var genericHeaders, genericItems int
	if err := db.QueryRowContext(ctx, `SELECT
    (SELECT COUNT(*) FROM settings_checkpoints),
    (SELECT COUNT(*) FROM settings_checkpoint_items)`).
		Scan(&genericHeaders, &genericItems); err != nil {
		t.Fatalf("count new settings history: %v", err)
	}
	if genericHeaders != 0 || genericItems != 0 {
		t.Fatalf("new history was backfilled: headers=%d items=%d", genericHeaders, genericItems)
	}
	assertSettingsCheckpointAuditLink(t, ctx, db, now)
	requireNoForeignKeyViolations(t, ctx, db)
}

func assertLegacySettingsHistory(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var action string
	var restoredFrom, auditCheckpoint, revision int64
	if err := db.QueryRowContext(ctx, `SELECT
    restored.action, restored.restored_from_id, audit.sync_config_checkpoint_id, item.revision
FROM sync_config_checkpoints restored
JOIN audit_entries audit ON audit.sync_config_checkpoint_id = restored.id
JOIN sync_config_checkpoint_items item ON item.checkpoint_id = restored.restored_from_id
WHERE restored.id = 42`).Scan(&action, &restoredFrom, &auditCheckpoint, &revision); err != nil {
		t.Fatalf("read preserved legacy settings history: %v", err)
	}
	if action != "restore" || restoredFrom != 41 || auditCheckpoint != 42 || revision != 7 {
		t.Fatalf("legacy history changed: action=%q source=%d audit=%d revision=%d",
			action, restoredFrom, auditCheckpoint, revision)
	}
}

func assertSettingsCheckpointAuditLink(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	now string,
) {
	t.Helper()

	if _, err := db.ExecContext(ctx, `INSERT INTO settings_checkpoints (
id, scope, target_id, actor_account_id, action, created_at
) VALUES (100, 'installation', 'installation:one', 'github:owner', 'save', ?)`, now); err != nil {
		t.Fatalf("insert generic settings checkpoint: %v", err)
	}
	insertAudit := `INSERT INTO audit_entries (
target_id, settings_checkpoint_id, actor_account_id, action, summary, created_at
) VALUES ('installation:one', 100, 'github:owner', 'settings.saved', 'Saved settings', ?)`
	if _, err := db.ExecContext(ctx, insertAudit, now); err != nil {
		t.Fatalf("link generic settings checkpoint audit: %v", err)
	}
	if _, err := db.ExecContext(ctx, insertAudit, now); err == nil {
		t.Fatal("linked one generic settings checkpoint to two target audit rows")
	}

	var generic, legacy int64
	if err := db.QueryRowContext(ctx, `SELECT
    (SELECT settings_checkpoint_id FROM audit_entries WHERE action = 'settings.saved'),
    (SELECT sync_config_checkpoint_id FROM audit_entries WHERE action = 'sync.config.restored')`).
		Scan(&generic, &legacy); err != nil {
		t.Fatalf("read both settings audit links: %v", err)
	}
	if generic != 100 || legacy != 42 {
		t.Fatalf("settings audit links = generic:%d legacy:%d", generic, legacy)
	}
}
