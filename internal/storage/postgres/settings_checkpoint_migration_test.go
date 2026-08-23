package postgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

// TestLegacySyncHistoryDropMigrationPreservesAuditRows is the production
// engine counterpart to SQLite's destructive migration proof.
func TestLegacySyncHistoryDropMigrationPreservesAuditRows(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SMYKLOT_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("SMYKLOT_TEST_POSTGRES_DSN is not set, so there is no server to migrate")
	}

	ctx := context.Background()
	scoped := scopedSchema(t, ctx, dsn)
	legacy, err := sqlstore.MigrationsBefore(migrations, 25)
	if err != nil {
		t.Fatalf("cut the migration series: %v", err)
	}
	db := openPool(t, ctx, scoped)
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlstore.Migrate(ctx, db, Dialect{}, legacy); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}
	now := time.Date(2026, time.August, 23, 8, 0, 0, 0, time.UTC)
	seedPostgresLegacySyncHistoryForDrop(t, ctx, db, now)

	if err := sqlstore.Migrate(ctx, db, Dialect{}, migrations); err != nil {
		t.Fatalf("apply legacy Sync history drop migration: %v", err)
	}

	var legacyTables int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM pg_catalog.pg_tables
WHERE schemaname = current_schema()
  AND tablename IN ('sync_config_checkpoints', 'sync_config_checkpoint_items')`).
		Scan(&legacyTables); err != nil {
		t.Fatalf("count legacy Sync history tables: %v", err)
	}
	if legacyTables != 0 {
		t.Fatalf("legacy Sync history tables remaining = %d", legacyTables)
	}

	var legacyColumn, canonicalColumn int
	if err := db.QueryRowContext(ctx, `SELECT
    COUNT(*) FILTER (WHERE column_name = 'sync_config_checkpoint_id'),
    COUNT(*) FILTER (WHERE column_name = 'settings_checkpoint_id')
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = 'audit_entries'`).
		Scan(&legacyColumn, &canonicalColumn); err != nil {
		t.Fatalf("read audit checkpoint columns: %v", err)
	}
	if legacyColumn != 0 || canonicalColumn != 1 {
		t.Fatalf("audit checkpoint columns = legacy:%d canonical:%d", legacyColumn, canonicalColumn)
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

	assertPostgresLegacyRootAuditPointerCleared(t, ctx, db)
	assertPostgresDiscardedSettingsAuditLinksCleared(t, ctx, db)
	assertPostgresSettingsCheckpointAuditLink(t, ctx, db, now)
}

func seedPostgresLegacySyncHistoryForDrop(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	now time.Time,
) {
	t.Helper()

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ('github:owner', 'github', 'owner', 'owner', 'Owner', $1)`, []any{now}},
		{`INSERT INTO targets (
id, installation_id, kind, account_id, settings_updated_at, synced_at
) VALUES ('installation:one', '1', 'Organization', 'github:owner', $1, $1)`, []any{now}},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed legacy Sync history: %v\n%s", err, statement.query)
		}
	}

	var checkpointID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO sync_config_checkpoints (
target_id, actor_account_id, action, created_at
) VALUES ('installation:one', 'github:owner', 'save', $1)
RETURNING id`, now).Scan(&checkpointID); err != nil {
		t.Fatalf("insert legacy Sync checkpoint: %v", err)
	}
	statements = []struct {
		query string
		args  []any
	}{
		{`INSERT INTO sync_config_checkpoint_items (
checkpoint_id, kind, enabled, document, digest, revision
) VALUES ($1, 'labels', true, '{"labels":[]}', 'legacy-digest', 7)`, []any{checkpointID}},
		{`INSERT INTO audit_entries (
target_id, sync_config_checkpoint_id, actor_account_id, action, summary, created_at
) VALUES ('installation:one', $1, 'github:owner', 'sync.config.restored',
          'Restored labels sync configuration', $2)`, []any{checkpointID, now}},
		{
			`INSERT INTO app_audit_events (
category, source_kind, source_id, target_id, actor_account_id, action, summary, created_at
) VALUES ('configuration', 'sync_config_checkpoint', $1, 'installation:one',
          'github:owner', 'sync.config.restored', 'Restored labels sync configuration', $2)`,
			[]any{checkpointID, now},
		},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed legacy Sync history: %v\n%s", err, statement.query)
		}
	}
	var settingsCheckpointID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO settings_checkpoints (
scope, target_id, actor_account_id, action, created_at
) VALUES ('installation', 'installation:one', 'github:owner', 'save', $1)
RETURNING id`, now).Scan(&settingsCheckpointID); err != nil {
		t.Fatalf("insert generic settings checkpoint: %v", err)
	}
	statements = []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO audit_entries (
target_id, settings_checkpoint_id, actor_account_id, action, summary, created_at
) VALUES ('installation:one', $1, 'github:owner', 'settings.saved', 'Saved settings', $2)`,
			[]any{settingsCheckpointID, now},
		},
		{
			`INSERT INTO app_audit_events (
category, source_kind, source_id, target_id, actor_account_id, action, summary, created_at
) VALUES ('configuration', 'settings_checkpoint', $1, 'installation:one',
          'github:owner', 'settings.saved', 'Saved settings', $2)`,
			[]any{settingsCheckpointID, now},
		},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed generic settings history: %v\n%s", err, statement.query)
		}
	}
}

func assertPostgresLegacyRootAuditPointerCleared(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
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

func assertPostgresDiscardedSettingsAuditLinksCleared(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
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

func assertPostgresSettingsCheckpointAuditLink(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	now time.Time,
) {
	t.Helper()

	var checkpointID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO settings_checkpoints (
scope, target_id, actor_account_id, action, created_at
) VALUES ('installation', 'installation:one', 'github:owner', 'save', $1)
RETURNING id`, now).Scan(&checkpointID); err != nil {
		t.Fatalf("insert generic settings checkpoint: %v", err)
	}
	insertAudit := `INSERT INTO audit_entries (
target_id, settings_checkpoint_id, actor_account_id, action, summary, created_at
) VALUES ('installation:one', $1, 'github:owner', 'settings.saved', 'Saved settings', $2)`
	if _, err := db.ExecContext(ctx, insertAudit, checkpointID, now); err != nil {
		t.Fatalf("link generic settings checkpoint audit: %v", err)
	}
	if _, err := db.ExecContext(ctx, insertAudit, checkpointID, now); err == nil {
		t.Fatal("linked one generic settings checkpoint to two target audit rows")
	}
}
