package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	baselineOwnerID  = "github:baseline-owner"
	baselineRootID   = "github:baseline-root"
	baselineTargetID = "installation:baseline-one"
	baselineEmptyID  = "installation:baseline-empty"
	baselineRepoID   = "repository:baseline-unavailable"
)

func TestOpenBackfillsCompleteSettingsBaselinesOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "settings-baseline.db")
	legacy := openLegacyDatabase(t, ctx, path, 39)
	seedSettingsBaselineFixture(t, ctx, legacy, false)
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open store with settings baseline backfill: %v", err)
	}
	assertCompleteSettingsBaselines(t, ctx, store)
	if err := store.Close(); err != nil {
		t.Fatalf("close first migrated store: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen store after settings baseline backfill: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertSettingsBaselineCounts(t, ctx, reopened.DB(), 3, 7)
}

func TestOpenRollsBackEverySettingsBaselineOnInvalidState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "invalid-settings-baseline.db")
	legacy := openLegacyDatabase(t, ctx, path, 39)
	seedSettingsBaselineFixture(t, ctx, legacy, true)
	if err := legacy.Close(); err != nil {
		t.Fatalf("close invalid legacy database: %v", err)
	}

	if _, err := Open(ctx, path); err == nil ||
		!strings.Contains(err.Error(), "sync config document must be valid JSON") {
		t.Fatalf("open invalid settings database error = %v", err)
	}

	raw, err := sql.Open("sqlite", dataSourceName(path))
	if err != nil {
		t.Fatalf("open failed-backfill database: %v", err)
	}
	defer func() { _ = raw.Close() }()
	assertSettingsBaselineCounts(t, ctx, raw, 0, 0)
}

func TestReconcileCreatesCompleteSettingsBaselineWithoutRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "reconcile-settings-baseline.db"))
	if err != nil {
		t.Fatalf("open empty store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Date(2026, time.August, 23, 11, 0, 0, 0, time.UTC)
	account := storage.Account{
		ID: baselineOwnerID, Provider: "github", SubjectID: "baseline-owner",
		Login: "owner", DisplayName: "Owner", UpdatedAt: now,
	}
	installation := storage.InstallationSnapshot{
		TargetID: baselineTargetID, InstallationID: "101",
		Kind: storage.TargetOrganization, Account: account,
		Repositories: []storage.RepositorySnapshot{
			{ID: baselineRepoID, Name: "retired", FullName: "owner/retired", DefaultBranch: "main"},
			{ID: "repository:baseline-current", Name: "current", FullName: "owner/current", DefaultBranch: "main"},
		},
		Ownership: storage.OwnershipSnapshot{
			Source: storage.OwnershipSourceOrganizationAdmin,
			Status: storage.OwnershipStatusFresh, Owners: []storage.Account{account}, SyncedAt: now,
		},
		SyncedAt: now,
	}
	if err := store.ReconcileInstallation(ctx, installation); err != nil {
		t.Fatalf("reconcile new installation: %v", err)
	}
	assertSettingsBaselineCounts(t, ctx, store.DB(), 1, 3)
	original := readSettingsBaseline(t, ctx, store, storage.SettingsCheckpointRef{
		Scope: storage.SettingsCheckpointScopeInstallation, TargetID: baselineTargetID,
	})
	if original.ActorAccountID != baselineOwnerID || len(original.Items) != 3 {
		t.Fatalf("reconciled baseline actor/items = %q/%d", original.ActorAccountID, len(original.Items))
	}

	installation.Repositories = installation.Repositories[1:]
	installation.SyncedAt = now.Add(time.Minute)
	if err := store.ReconcileInstallation(ctx, installation); err != nil {
		t.Fatalf("reconcile installation again: %v", err)
	}
	assertSettingsBaselineCounts(t, ctx, store.DB(), 1, 3)
	unchanged := readSettingsBaseline(t, ctx, store, storage.SettingsCheckpointRef{
		Scope: storage.SettingsCheckpointScopeInstallation, TargetID: baselineTargetID,
	})
	if !reflect.DeepEqual(original, unchanged) {
		t.Fatalf("reconcile mutated immutable baseline:\noriginal=%#v\ncurrent=%#v", original, unchanged)
	}

	second := installation
	second.TargetID = baselineEmptyID
	second.InstallationID = "102"
	second.Kind = storage.TargetUser
	second.Repositories = nil
	if err := store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{installation, second}); err != nil {
		t.Fatalf("reconcile catalog with new installation: %v", err)
	}
	assertSettingsBaselineCounts(t, ctx, store.DB(), 2, 4)
}

func seedSettingsBaselineFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	invalidSyncDocument bool,
) {
	t.Helper()

	now := time.Date(2026, time.August, 23, 9, 0, 0, 0, time.UTC).
		Format(time.RFC3339Nano)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES (?, 'github', ?, ?, ?, ?)`, []any{baselineOwnerID, "baseline-owner", "owner", "Owner", now}},
		{`INSERT INTO accounts (id, provider, subject_id, login, display_name, updated_at)
VALUES (?, 'github', ?, ?, ?, ?)`, []any{baselineRootID, "baseline-root", "root", "Root", now}},
		{`INSERT INTO targets (
id, installation_id, kind, account_id, available,
repository_default_enabled, pending_ci_mode_default,
pending_ci_branch_patterns_default, pending_ci_quiet_period_seconds_override,
path_index_interval_seconds_override, config_patch, revision,
settings_updated_at, synced_at
) VALUES (?, '101', 'Organization', ?, 0, 1, 'labels',
          '{"include":["refs/heads/main"],"exclude":[]}', 17, 19,
          '{"quiet_success":true}', 8, ?, ?)`, []any{baselineTargetID, baselineOwnerID, now, now}},
		{`INSERT INTO targets (
id, installation_id, kind, account_id, settings_updated_at, synced_at
) VALUES (?, '102', 'User', ?, ?, ?)`, []any{baselineEmptyID, baselineRootID, now, now}},
		{
			`INSERT INTO repositories (
id, target_id, name, full_name, private, default_branch, available,
enabled_override, pending_ci_mode_override,
pending_ci_branch_patterns_override, pending_ci_quiet_period_seconds_override,
path_index_interval_seconds_override, config_patch, ignore_repository_file,
revision, settings_updated_at, synced_at
) VALUES (?, ?, 'retired', 'owner/retired', 1, 'main', 0,
          0, 'checks', '{"include":["refs/heads/release"],"exclude":[]}',
          23, 29, '{"command_prefix":"!"}', 1, 6, ?, ?)`,
			[]any{baselineRepoID, baselineTargetID, now, now},
		},
		{`INSERT INTO runtime_settings (
singleton, log_level, poll_interval_seconds, pending_ci_quiet_period_seconds,
session_ttl_seconds, path_index_interval_seconds, revision, updated_at,
updated_by_account_id
) VALUES (1, 'debug', 30, 12, 3600, 90, 4, ?, ?)`, []any{now, baselineRootID}},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed settings baseline fixture: %v\n%s", err, statement.query)
		}
	}
	seedSettingsBaselineSync(t, ctx, db, now, invalidSyncDocument)
}

func seedSettingsBaselineSync(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	now string,
	invalidDocument bool,
) {
	t.Helper()

	labels := []byte(`{"labels":[{"name":"ci/long"}]}`)
	if invalidDocument {
		labels = []byte(`{"labels":`)
	}
	configs := []struct {
		kind     orgsync.Kind
		enabled  bool
		document []byte
		revision int64
	}{
		{orgsync.KindLabels, true, labels, 5},
		{orgsync.KindFiles, false, []byte(`{"files":[]}`), 3},
	}
	for _, config := range configs {
		_, err := db.ExecContext(ctx, `INSERT INTO sync_configs (
target_id, kind, enabled, document, digest, revision, updated_by, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, baselineTargetID, config.kind, config.enabled,
			config.document, orgsync.DigestConfig(config.enabled, config.document),
			config.revision, baselineOwnerID, now)
		if err != nil {
			t.Fatalf("seed sync config: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sync_repository_overrides (
repository_id, kind, enabled_override, document, revision, updated_by, updated_at
) VALUES (?, 'files', 1, '{"templates":[]}', 7, ?, ?)`,
		baselineRepoID, baselineOwnerID, now); err != nil {
		t.Fatalf("seed sync override: %v", err)
	}
}

func assertCompleteSettingsBaselines(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()

	assertSettingsBaselineCounts(t, ctx, store.DB(), 3, 7)
	assertNoSettingsBaselineAudit(t, ctx, store.DB())

	checkpoint := readSettingsBaseline(t, ctx, store, storage.SettingsCheckpointRef{
		Scope: storage.SettingsCheckpointScopeInstallation, TargetID: baselineTargetID,
	})
	if checkpoint.ActorAccountID != baselineOwnerID || len(checkpoint.Items) != 5 {
		t.Fatalf("installation baseline actor/items = %q/%d", checkpoint.ActorAccountID, len(checkpoint.Items))
	}
	assertBaselineStates(t, checkpoint)
	assertTargetBaselineDocument(t, checkpoint)
	assertRepositoryBaselineDocument(t, checkpoint)
	assertSyncBaselineDocuments(t, checkpoint)

	empty := readSettingsBaseline(t, ctx, store, storage.SettingsCheckpointRef{
		Scope: storage.SettingsCheckpointScopeInstallation, TargetID: baselineEmptyID,
	})
	if empty.ActorAccountID != baselineRootID || len(empty.Items) != 1 ||
		empty.Items[0].Kind != storage.SettingsCheckpointItemTarget {
		t.Fatalf("empty installation baseline = %#v", empty)
	}

	root := readSettingsBaseline(t, ctx, store, storage.SettingsCheckpointRef{
		Scope: storage.SettingsCheckpointScopeRoot,
	})
	if root.ActorAccountID != baselineRootID || len(root.Items) != 1 {
		t.Fatalf("root baseline actor/items = %q/%d", root.ActorAccountID, len(root.Items))
	}
	var document storage.RuntimeSettingsDocument
	decodeBaselineDocument(t, root.Items[0], &document)
	if root.Items[0].Kind != storage.SettingsCheckpointItemRuntime ||
		document.LogLevel == nil || *document.LogLevel != "debug" ||
		document.PathIndexInterval == nil || *document.PathIndexInterval != 90*time.Second {
		t.Fatalf("runtime baseline document = %#v", document)
	}
}

func readSettingsBaseline(
	t *testing.T,
	ctx context.Context,
	store *Store,
	ref storage.SettingsCheckpointRef,
) storage.SettingsCheckpoint {
	t.Helper()

	var inspection storage.SettingsCheckpointInspection
	var err error
	if ref.Scope == storage.SettingsCheckpointScopeRoot {
		inspection, err = store.InspectRootSettingsBaseline(ctx)
	} else {
		inspection, err = store.InspectInstallationSettingsBaseline(ctx, ref.TargetID)
	}
	if err != nil {
		t.Fatalf("read settings baseline: %v", err)
	}
	checkpoint := inspection.Checkpoint
	if checkpoint.Action != storage.SettingsCheckpointActionBaseline ||
		checkpoint.RestoredFromID != nil {
		t.Fatalf("settings baseline action/source = %q/%v", checkpoint.Action, checkpoint.RestoredFromID)
	}

	return checkpoint
}

func assertBaselineStates(t *testing.T, checkpoint storage.SettingsCheckpoint) {
	t.Helper()

	for _, item := range checkpoint.Items {
		if item.Before != nil || item.After == nil {
			t.Fatalf("baseline item has wrong state sides: %#v", item)
		}
		if item.After.Digest != storage.DigestSettingsCheckpointDocument(item.After.Document) {
			t.Fatalf("baseline item %q digest does not match", item.Kind)
		}
	}
}

func assertTargetBaselineDocument(t *testing.T, checkpoint storage.SettingsCheckpoint) {
	t.Helper()

	item := findBaselineItem(t, checkpoint, storage.SettingsCheckpointItemTarget, "", "")
	var document storage.TargetSettingsDocument
	decodeBaselineDocument(t, item, &document)
	if item.After.Revision != 8 || !document.RepositoryDefaultEnabled ||
		document.PendingCIModeDefault != storage.PendingCIModeLabels ||
		document.PendingCIQuietPeriodOverride == nil ||
		*document.PendingCIQuietPeriodOverride != 17*time.Second {
		t.Fatalf("target baseline document = %#v at revision %d", document, item.After.Revision)
	}
}

func assertRepositoryBaselineDocument(t *testing.T, checkpoint storage.SettingsCheckpoint) {
	t.Helper()

	item := findBaselineItem(
		t, checkpoint, storage.SettingsCheckpointItemRepository, baselineRepoID, "",
	)
	var document storage.RepositorySettingsDocument
	decodeBaselineDocument(t, item, &document)
	if item.RepositoryFullName != "owner/retired" || item.After.Revision != 6 ||
		document.EnabledOverride == nil || *document.EnabledOverride ||
		!document.IgnoreRepositoryFile || document.PathIndexIntervalOverride == nil ||
		*document.PathIndexIntervalOverride != 29*time.Second {
		t.Fatalf("repository baseline document = %#v at revision %d", document, item.After.Revision)
	}
}

func assertSyncBaselineDocuments(t *testing.T, checkpoint storage.SettingsCheckpoint) {
	t.Helper()

	labels := findBaselineItem(
		t, checkpoint, storage.SettingsCheckpointItemSyncConfig, "", orgsync.KindLabels,
	)
	var config storage.SyncConfigSettingsDocument
	decodeBaselineDocument(t, labels, &config)
	if labels.After.Revision != 5 || !config.Enabled ||
		config.Document != `{"labels":[{"name":"ci/long"}]}` {
		t.Fatalf("labels baseline document = %#v at revision %d", config, labels.After.Revision)
	}

	override := findBaselineItem(
		t, checkpoint, storage.SettingsCheckpointItemSyncOverride,
		baselineRepoID, orgsync.KindFiles,
	)
	var document storage.SyncOverrideSettingsDocument
	decodeBaselineDocument(t, override, &document)
	if override.After.Revision != 7 || document.Enabled == nil || !*document.Enabled ||
		document.Document != `{"templates":[]}` {
		t.Fatalf("sync override baseline document = %#v at revision %d", document, override.After.Revision)
	}
}

func findBaselineItem(
	t *testing.T,
	checkpoint storage.SettingsCheckpoint,
	kind storage.SettingsCheckpointItemKind,
	repositoryID string,
	syncKind orgsync.Kind,
) storage.SettingsCheckpointItem {
	t.Helper()

	for _, item := range checkpoint.Items {
		if item.Kind == kind && item.RepositoryID == repositoryID && item.SyncKind == syncKind {
			return item
		}
	}
	t.Fatalf("baseline item %q/%q/%q is missing", kind, repositoryID, syncKind)

	return storage.SettingsCheckpointItem{}
}

func decodeBaselineDocument(t *testing.T, item storage.SettingsCheckpointItem, target any) {
	t.Helper()

	if item.After == nil {
		t.Fatalf("baseline item %q has no after state", item.Kind)
	}
	if err := json.Unmarshal(item.After.Document, target); err != nil {
		t.Fatalf("decode %q baseline document: %v", item.Kind, err)
	}
}

func assertSettingsBaselineCounts(
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
		t.Fatalf("count settings baselines: %v", err)
	}
	if gotHeaders != headers || gotItems != items {
		t.Fatalf("settings baseline counts = %d/%d, want %d/%d",
			gotHeaders, gotItems, headers, items)
	}
}

func assertNoSettingsBaselineAudit(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var targetAudit, rootAudit int
	if err := db.QueryRowContext(ctx, `SELECT
    (SELECT COUNT(*) FROM audit_entries WHERE settings_checkpoint_id IS NOT NULL),
    (SELECT COUNT(*) FROM app_audit_events)`).Scan(&targetAudit, &rootAudit); err != nil {
		t.Fatalf("count settings baseline audit: %v", err)
	}
	if targetAudit != 0 || rootAudit != 0 {
		t.Fatalf("settings baseline wrote audit rows: target=%d root=%d", targetAudit, rootAudit)
	}
}
