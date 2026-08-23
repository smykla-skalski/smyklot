package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/sqlstore"
)

func TestOpenBackfillsPostgresSettingsBaselinesOnce(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("SMYKLOT_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("SMYKLOT_TEST_POSTGRES_DSN is not set, so there is no server to migrate")
	}

	ctx := context.Background()
	scoped := scopedSchema(t, ctx, dsn)
	legacyMigrations, err := sqlstore.MigrationsBefore(migrations, 24)
	if err != nil {
		t.Fatalf("cut the migration series: %v", err)
	}
	legacy := openPool(t, ctx, scoped)
	if err := sqlstore.Migrate(ctx, legacy, Dialect{}, legacyMigrations); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}
	seedPostgresSettingsBaselineFixture(t, ctx, legacy)
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := Open(ctx, scoped)
	if err != nil {
		t.Fatalf("open store with settings baseline backfill: %v", err)
	}
	assertPostgresSettingsBaselines(t, ctx, store.DB())
	if err := store.Close(); err != nil {
		t.Fatalf("close first migrated store: %v", err)
	}

	reopened, err := Open(ctx, scoped)
	if err != nil {
		t.Fatalf("reopen store after settings baseline backfill: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertPostgresSettingsBaselineCounts(t, ctx, reopened.DB(), 2, 5)
}

func seedPostgresSettingsBaselineFixture(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	const (
		accountID = "github:postgres-baseline"
		targetID  = "installation:postgres-baseline"
		repoID    = "repository:postgres-baseline"
	)
	now := time.Date(2026, time.August, 23, 9, 30, 0, 0, time.UTC)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES ($1, 'github', 'postgres-baseline', 'owner', 'Owner', $2)`, []any{accountID, now}},
		{`INSERT INTO targets (
id, installation_id, kind, account_id, repository_default_enabled,
pending_ci_mode_default, pending_ci_branch_patterns_default,
pending_ci_quiet_period_seconds_override, path_index_interval_seconds_override,
config_patch, revision, settings_updated_at, synced_at
) VALUES ($1, '201', 'Organization', $2, true, 'labels',
          '{"include":["refs/heads/main"],"exclude":[]}', 31, 37,
          '{"quiet_pending":true}', 9, $3, $3)`, []any{targetID, accountID, now}},
		{`INSERT INTO repositories (
id, target_id, name, full_name, private, default_branch, available,
enabled_override, config_patch, ignore_repository_file,
revision, settings_updated_at, synced_at
) VALUES ($1, $2, 'gone', 'owner/gone', true, 'main', false,
          false, '{"command_prefix":"?"}', true, 11, $3, $3)`, []any{repoID, targetID, now}},
		{`INSERT INTO runtime_settings (
singleton, log_level, poll_interval_seconds, pending_ci_quiet_period_seconds,
session_ttl_seconds, path_index_interval_seconds, revision, updated_at,
updated_by_account_id
) VALUES (1, 'warn', 45, 14, 7200, 120, 5, $1, $2)`, []any{now, accountID}},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed PostgreSQL settings baseline fixture: %v\n%s", err, statement.query)
		}
	}
	document := []byte(`{"labels":[]}`)
	if _, err := db.ExecContext(ctx, `INSERT INTO sync_configs (
target_id, kind, enabled, document, digest, revision, updated_by, updated_at
) VALUES ($1, 'labels', true, $2, $3, 13, $4, $5)`, targetID, string(document),
		orgsync.DigestConfig(true, document), accountID, now); err != nil {
		t.Fatalf("seed PostgreSQL sync config: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sync_repository_overrides (
repository_id, kind, enabled_override, document, revision, updated_by, updated_at
) VALUES ($1, 'files', true, '{"templates":[]}', 17, $2, $3)`,
		repoID, accountID, now); err != nil {
		t.Fatalf("seed PostgreSQL sync override: %v", err)
	}
}

func assertPostgresSettingsBaselines(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	assertPostgresSettingsBaselineCounts(t, ctx, db, 2, 5)
	rows, err := db.QueryContext(ctx, `SELECT
    item.item_kind, item.before_document, item.after_document, item.after_digest
FROM settings_checkpoint_items item
JOIN settings_checkpoints checkpoint ON checkpoint.id = item.checkpoint_id
WHERE checkpoint.action = 'baseline'
ORDER BY checkpoint.scope, item.item_kind`)
	if err != nil {
		t.Fatalf("list PostgreSQL settings baseline items: %v", err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[storage.SettingsCheckpointItemKind]bool{}
	for rows.Next() {
		var kind storage.SettingsCheckpointItemKind
		var before sql.NullString
		var document []byte
		var digest string
		if err := rows.Scan(&kind, &before, &document, &digest); err != nil {
			t.Fatalf("scan PostgreSQL settings baseline item: %v", err)
		}
		if before.Valid || digest != storage.DigestSettingsCheckpointDocument(document) {
			t.Fatalf("PostgreSQL baseline %q has before state or wrong digest", kind)
		}
		seen[kind] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read PostgreSQL settings baseline items: %v", err)
	}
	for _, kind := range []storage.SettingsCheckpointItemKind{
		storage.SettingsCheckpointItemTarget,
		storage.SettingsCheckpointItemRepository,
		storage.SettingsCheckpointItemSyncConfig,
		storage.SettingsCheckpointItemSyncOverride,
		storage.SettingsCheckpointItemRuntime,
	} {
		if !seen[kind] {
			t.Fatalf("PostgreSQL settings baseline is missing %q", kind)
		}
	}
	assertPostgresSettingsBaselineDocuments(t, ctx, db)
}

func assertPostgresSettingsBaselineDocuments(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var repositoryDocument, runtimeDocument []byte
	if err := db.QueryRowContext(ctx, `SELECT
    (SELECT item.after_document FROM settings_checkpoint_items item
     WHERE item.item_kind = 'repository'),
    (SELECT item.after_document FROM settings_checkpoint_items item
     WHERE item.item_kind = 'runtime')`).Scan(&repositoryDocument, &runtimeDocument); err != nil {
		t.Fatalf("read PostgreSQL settings baseline documents: %v", err)
	}
	var repository storage.RepositorySettingsDocument
	if err := json.Unmarshal(repositoryDocument, &repository); err != nil {
		t.Fatalf("decode PostgreSQL repository baseline: %v", err)
	}
	var runtime storage.RuntimeSettingsDocument
	if err := json.Unmarshal(runtimeDocument, &runtime); err != nil {
		t.Fatalf("decode PostgreSQL runtime baseline: %v", err)
	}
	if !repository.IgnoreRepositoryFile || repository.EnabledOverride == nil ||
		*repository.EnabledOverride || runtime.LogLevel == nil || *runtime.LogLevel != "warn" ||
		runtime.PathIndexInterval == nil || *runtime.PathIndexInterval != 120*time.Second {
		t.Fatalf("PostgreSQL baseline documents = repository:%#v runtime:%#v", repository, runtime)
	}
	var targetAudit, rootAudit int
	if err := db.QueryRowContext(ctx, `SELECT
    (SELECT COUNT(*) FROM audit_entries WHERE settings_checkpoint_id IS NOT NULL),
    (SELECT COUNT(*) FROM app_audit_events)`).Scan(&targetAudit, &rootAudit); err != nil {
		t.Fatalf("count PostgreSQL settings baseline audit: %v", err)
	}
	if targetAudit != 0 || rootAudit != 0 {
		t.Fatalf("PostgreSQL settings baseline wrote audit: target=%d root=%d", targetAudit, rootAudit)
	}
}

func assertPostgresSettingsBaselineCounts(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	headers, items int,
) {
	t.Helper()

	var gotHeaders, gotItems int
	if err := db.QueryRowContext(ctx, `SELECT
    (SELECT COUNT(*) FROM settings_checkpoints WHERE action = 'baseline'),
    (SELECT COUNT(*) FROM settings_checkpoint_items item
     JOIN settings_checkpoints checkpoint ON checkpoint.id = item.checkpoint_id
     WHERE checkpoint.action = 'baseline')`).Scan(&gotHeaders, &gotItems); err != nil {
		t.Fatalf("count PostgreSQL settings baselines: %v", err)
	}
	if gotHeaders != headers || gotItems != items {
		t.Fatalf("PostgreSQL settings baseline counts = %d/%d, want %d/%d",
			gotHeaders, gotItems, headers, items)
	}
}
