package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// TestLegacySyncHistoryDropMigrationPreservesAuditRows proves the clean cut
// removes only the superseded snapshot schema. Human-readable target and Root
// audit history remains, while the Root event no longer points at a dropped row.
func TestLegacySyncHistoryDropMigrationPreservesAuditRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "drop-sync-history.db")
	db := openLegacyDatabase(t, ctx, path, 40)
	now := time.Date(2026, time.August, 23, 8, 0, 0, 0, time.UTC).
		Format(time.RFC3339Nano)

	seedLegacySyncHistoryForDrop(t, ctx, db, now)
	if err := sqlstore.Migrate(ctx, db, Dialect{}, migrations); err != nil {
		t.Fatalf("apply legacy Sync history drop migration: %v", err)
	}

	for _, table := range []string{"sync_config_checkpoints", "sync_config_checkpoint_items"} {
		var count int
		if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).
			Scan(&count); err != nil {
			t.Fatalf("check dropped table %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("legacy table %s remains", table)
		}
	}
	columns := tableColumns(t, ctx, db, "audit_entries")
	if columns["sync_config_checkpoint_id"] || !columns["settings_checkpoint_id"] {
		t.Fatalf("audit checkpoint columns after clean cut = %#v", columns)
	}

	var targetAuditRows int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM audit_entries WHERE action = 'sync.config.restored'`).
		Scan(&targetAuditRows); err != nil {
		t.Fatalf("count preserved target audit row: %v", err)
	}
	if targetAuditRows != 1 {
		t.Fatalf("preserved target audit rows = %d, want 1", targetAuditRows)
	}

	assertLegacyRootAuditPointerCleared(t, ctx, db)
	assertDiscardedSettingsAuditLinksCleared(t, ctx, db)
	assertSettingsCheckpointAuditLink(t, ctx, db, now)
	requireNoForeignKeyViolations(t, ctx, db)
}

func seedLegacySyncHistoryForDrop(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	now string,
) {
	t.Helper()

	statements := []string{
		`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ('github:owner', 'github', 'owner', 'owner', 'Owner', ?)`,
		`INSERT INTO targets (
id, installation_id, kind, account_id, settings_updated_at, synced_at
) VALUES ('installation:one', '1', 'Organization', 'github:owner', ?, ?)`,
		`INSERT INTO sync_config_checkpoints (
id, target_id, actor_account_id, action, created_at
) VALUES (41, 'installation:one', 'github:owner', 'save', ?)`,
		`INSERT INTO sync_config_checkpoint_items (
checkpoint_id, kind, enabled, document, digest, revision
) VALUES (41, 'labels', 1, '{"labels":[]}', 'legacy-digest', 7)`,
		`INSERT INTO audit_entries (
target_id, sync_config_checkpoint_id, actor_account_id, action, summary, created_at
) VALUES ('installation:one', 41, 'github:owner', 'sync.config.restored',
          'Restored labels sync configuration', ?)`,
		`INSERT INTO app_audit_events (
category, source_kind, source_id, target_id, actor_account_id, action, summary, created_at
) VALUES ('configuration', 'sync_config_checkpoint', 41, 'installation:one',
          'github:owner', 'sync.config.restored', 'Restored labels sync configuration', ?)`,
		`INSERT INTO settings_checkpoints (
id, scope, target_id, actor_account_id, action, created_at
) VALUES (90, 'installation', 'installation:one', 'github:owner', 'save', ?)`,
		`INSERT INTO audit_entries (
target_id, settings_checkpoint_id, actor_account_id, action, summary, created_at
) VALUES ('installation:one', 90, 'github:owner', 'settings.saved', 'Saved settings', ?)`,
		`INSERT INTO app_audit_events (
category, source_kind, source_id, target_id, actor_account_id, action, summary, created_at
) VALUES ('configuration', 'settings_checkpoint', 90, 'installation:one',
          'github:owner', 'settings.saved', 'Saved settings', ?)`,
	}
	arguments := [][]any{{now}, {now, now}, {now}, nil, {now}, {now}, {now}, {now}, {now}}
	for index, statement := range statements {
		if _, err := db.ExecContext(ctx, statement, arguments[index]...); err != nil {
			t.Fatalf("seed legacy Sync history: %v\n%s", err, statement)
		}
	}
}

func assertLegacyRootAuditPointerCleared(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var sourceKind sql.NullString
	var sourceID sql.NullInt64
	if err := db.QueryRowContext(ctx, `
SELECT source_kind, source_id FROM app_audit_events
WHERE action = 'sync.config.restored'`).Scan(&sourceKind, &sourceID); err != nil {
		t.Fatalf("read preserved legacy Root audit row: %v", err)
	}
	if sourceKind.Valid || sourceID.Valid {
		t.Fatalf("legacy Root audit pointer remains: kind=%#v id=%#v", sourceKind, sourceID)
	}
}

func assertDiscardedSettingsAuditLinksCleared(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var sourceKind sql.NullString
	var sourceID sql.NullInt64
	if err := db.QueryRowContext(ctx, `
SELECT source_kind, source_id FROM app_audit_events
WHERE action = 'settings.saved'`).Scan(&sourceKind, &sourceID); err != nil {
		t.Fatalf("read preserved Root settings audit row: %v", err)
	}
	if sourceKind.Valid || sourceID.Valid {
		t.Fatalf("discarded Root settings pointer remains: kind=%#v id=%#v", sourceKind, sourceID)
	}
	var checkpointID sql.NullInt64
	if err := db.QueryRowContext(ctx, `
SELECT settings_checkpoint_id FROM audit_entries
WHERE action = 'settings.saved'`).Scan(&checkpointID); err != nil {
		t.Fatalf("read preserved target settings audit row: %v", err)
	}
	if checkpointID.Valid {
		t.Fatalf("discarded target settings pointer remains: %#v", checkpointID)
	}
	var checkpoints int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings_checkpoints`).
		Scan(&checkpoints); err != nil {
		t.Fatalf("count discarded settings checkpoints: %v", err)
	}
	if checkpoints != 0 {
		t.Fatalf("discarded settings checkpoints remaining = %d", checkpoints)
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
}
