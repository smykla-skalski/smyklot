package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type settingsBaselineTarget struct {
	id             string
	actorAccountID string
}

// BackfillSettingsCheckpointBaselines captures the complete settings state
// that existed when generic history was introduced. It is deliberately run by
// each adapter immediately after migrations and before the store is returned.
func (s *Store) BackfillSettingsCheckpointBaselines(
	ctx context.Context,
	createdAt time.Time,
) error {
	if createdAt.IsZero() {
		return errors.New("settings baseline creation time is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin settings baseline backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := listSettingsBaselineTargets(ctx, tx)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := s.backfillInstallationSettingsBaseline(ctx, tx, target, createdAt); err != nil {
			return err
		}
	}
	if err := s.backfillRuntimeSettingsBaseline(ctx, tx, createdAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit settings baseline backfill: %w", err)
	}

	return nil
}

func listSettingsBaselineTargets(
	ctx context.Context,
	queryer runner,
) ([]settingsBaselineTarget, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT id, account_id
FROM targets
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list settings baseline targets: %w", err)
	}
	targets, err := collectRows(rows, func(scanner rowScanner) (settingsBaselineTarget, error) {
		var target settingsBaselineTarget
		err := scanner.Scan(&target.id, &target.actorAccountID)

		return target, err
	})
	if err != nil {
		return nil, fmt.Errorf("read settings baseline targets: %w", err)
	}

	return targets, nil
}

func (s *Store) backfillInstallationSettingsBaseline(
	ctx context.Context,
	tx *transaction,
	target settingsBaselineTarget,
	createdAt time.Time,
) error {
	exists, err := settingsBaselineExists(
		ctx, tx, storage.SettingsCheckpointScopeInstallation, target.id,
	)
	if err != nil || exists {
		return err
	}
	items, err := installationSettingsBaselineItems(ctx, tx, target.id)
	if err != nil {
		return fmt.Errorf("build settings baseline for installation %q: %w", target.id, err)
	}
	err = insertSettingsBaseline(ctx, tx, storage.SettingsCheckpointCreate{
		Scope: storage.SettingsCheckpointScopeInstallation, TargetID: target.id,
		ActorAccountID: target.actorAccountID,
		Action:         storage.SettingsCheckpointActionBaseline,
		CreatedAt:      createdAt,
		Items:          items,
	})
	if err != nil {
		return fmt.Errorf("write settings baseline for installation %q: %w", target.id, err)
	}

	return nil
}

func installationSettingsBaselineItems(
	ctx context.Context,
	tx *transaction,
	targetID string,
) ([]storage.SettingsCheckpointItem, error) {
	target, err := readSettingsBaselineTarget(ctx, tx, targetID)
	if err != nil {
		return nil, fmt.Errorf("read target settings: %w", err)
	}
	state, err := targetSettingsState(target.document, target.revision)
	if err != nil {
		return nil, err
	}
	items := []storage.SettingsCheckpointItem{{
		Kind:            storage.SettingsCheckpointItemTarget,
		DocumentVersion: storage.SettingsCheckpointDocumentVersion, After: state,
	}}
	repositories, err := listSettingsBaselineRepositories(ctx, tx, targetID)
	if err != nil {
		return nil, err
	}
	items, repositoriesByID, err := appendRepositoryBaselineItems(items, repositories)
	if err != nil {
		return nil, err
	}
	items, err = appendSyncConfigBaselineItems(ctx, tx, targetID, items)
	if err != nil {
		return nil, err
	}

	return appendSyncOverrideBaselineItems(ctx, tx, targetID, repositoriesByID, items)
}

func appendRepositoryBaselineItems(
	items []storage.SettingsCheckpointItem,
	repositories []settingsBaselineRepository,
) ([]storage.SettingsCheckpointItem, map[string]string, error) {
	byID := make(map[string]string, len(repositories))
	for _, repository := range repositories {
		state, err := repositorySettingsState(
			repository.document, repository.revision,
		)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, storage.SettingsCheckpointItem{
			Kind:         storage.SettingsCheckpointItemRepository,
			RepositoryID: repository.id, RepositoryFullName: repository.fullName,
			DocumentVersion: storage.SettingsCheckpointDocumentVersion, After: state,
		})
		byID[repository.id] = repository.fullName
	}

	return items, byID, nil
}

func appendSyncConfigBaselineItems(
	ctx context.Context,
	queryer runner,
	targetID string,
	items []storage.SettingsCheckpointItem,
) ([]storage.SettingsCheckpointItem, error) {
	configs, err := listSyncConfigs(ctx, queryer, targetID)
	if err != nil {
		return nil, err
	}
	for _, config := range configs {
		if err := validateStoredSyncConfig(config); err != nil {
			return nil, err
		}
		state, err := syncConfigSettingsState(
			syncConfigSettingsDocument(config), config.Revision,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, storage.SettingsCheckpointItem{
			Kind: storage.SettingsCheckpointItemSyncConfig, SyncKind: config.Kind,
			DocumentVersion: storage.SettingsCheckpointDocumentVersion, After: state,
		})
	}

	return items, nil
}

func appendSyncOverrideBaselineItems(
	ctx context.Context,
	queryer runner,
	targetID string,
	repositories map[string]string,
	items []storage.SettingsCheckpointItem,
) ([]storage.SettingsCheckpointItem, error) {
	overrides, err := listSettingsBaselineSyncOverrides(ctx, queryer, targetID)
	if err != nil {
		return nil, err
	}
	for _, override := range overrides {
		if !json.Valid(override.Document) {
			return nil, fmt.Errorf("stored %s sync override document must be valid JSON", override.Kind)
		}
		fullName, ok := repositories[override.RepositoryID]
		if !ok {
			return nil, fmt.Errorf("sync override repository %q is missing", override.RepositoryID)
		}
		state, err := syncOverrideSettingsState(
			syncOverrideSettingsDocument(override), override.Revision,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, storage.SettingsCheckpointItem{
			Kind:         storage.SettingsCheckpointItemSyncOverride,
			RepositoryID: override.RepositoryID, RepositoryFullName: fullName,
			SyncKind:        override.Kind,
			DocumentVersion: storage.SettingsCheckpointDocumentVersion, After: state,
		})
	}

	return items, nil
}

func listSettingsBaselineSyncOverrides(
	ctx context.Context,
	queryer runner,
	targetID string,
) ([]orgsync.RepositoryOverride, error) {
	rows, err := queryer.QueryContext(ctx, syncOverrideFrom+`
WHERE r.target_id = ?
ORDER BY o.repository_id, o.kind`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list sync overrides: %w", err)
	}
	overrides, err := collectRows(rows, scanSyncOverride)
	if err != nil {
		return nil, fmt.Errorf("read sync overrides: %w", err)
	}

	return overrides, nil
}

func (s *Store) backfillRuntimeSettingsBaseline(
	ctx context.Context,
	tx *transaction,
	createdAt time.Time,
) error {
	exists, err := settingsBaselineExists(ctx, tx, storage.SettingsCheckpointScopeRoot, "")
	if err != nil || exists {
		return err
	}
	settings, err := getRuntimeSettings(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read runtime settings baseline: %w", err)
	}
	if settings.UpdatedBy == nil {
		return errors.New("runtime settings baseline actor is missing")
	}
	state, err := runtimeSettingsState(runtimeSettingsDocument(settings), settings.Revision)
	if err != nil {
		return err
	}
	err = insertSettingsBaseline(ctx, tx, storage.SettingsCheckpointCreate{
		Scope:          storage.SettingsCheckpointScopeRoot,
		ActorAccountID: settings.UpdatedBy.ID,
		Action:         storage.SettingsCheckpointActionBaseline,
		CreatedAt:      createdAt,
		Items: []storage.SettingsCheckpointItem{{
			Kind:            storage.SettingsCheckpointItemRuntime,
			DocumentVersion: storage.SettingsCheckpointDocumentVersion, After: state,
		}},
	})
	if err != nil {
		return fmt.Errorf("write runtime settings baseline: %w", err)
	}

	return nil
}

func runtimeSettingsDocument(settings storage.RuntimeSettings) storage.RuntimeSettingsDocument {
	return storage.RuntimeSettingsDocument{
		BackgroundWorkPaused: settings.BackgroundWorkPaused,
		BotConfig:            settings.BotConfig, LogLevel: settings.LogLevel,
		PollInterval:         settings.PollInterval,
		PendingCIQuietPeriod: settings.PendingCIQuietPeriod,
		SessionTTL:           settings.SessionTTL,
		PathIndexInterval:    settings.PathIndexInterval,
	}
}

func runtimeSettingsState(
	document storage.RuntimeSettingsDocument,
	revision int64,
) (*storage.SettingsCheckpointState, error) {
	return installationSettingsState(document, revision)
}

func settingsBaselineExists(
	ctx context.Context,
	queryer rowQuerier,
	scope storage.SettingsCheckpointScope,
	targetID string,
) (bool, error) {
	var value int
	var err error
	if scope == storage.SettingsCheckpointScopeRoot {
		err = queryer.QueryRowContext(ctx, `
SELECT 1 FROM settings_checkpoints
WHERE scope = ? AND target_id IS NULL AND action = 'baseline'`, scope).Scan(&value)
	} else {
		err = queryer.QueryRowContext(ctx, `
SELECT 1 FROM settings_checkpoints
WHERE scope = ? AND target_id = ? AND action = 'baseline'`, scope, targetID).Scan(&value)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read settings baseline: %w", err)
	}

	return true, nil
}

func insertSettingsBaseline(
	ctx context.Context,
	tx *transaction,
	create storage.SettingsCheckpointCreate,
) error {
	if err := create.Validate(); err != nil {
		return err
	}
	var targetID any
	if create.TargetID != "" {
		targetID = create.TargetID
	}
	var checkpointID int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO settings_checkpoints (
    scope, target_id, actor_account_id, action, restored_from_id, created_at
) VALUES (?, ?, ?, ?, NULL, ?)
ON CONFLICT DO NOTHING
RETURNING id`, create.Scope, targetID, create.ActorAccountID, create.Action,
		create.CreatedAt).Scan(&checkpointID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert settings baseline: %w", err)
	}

	items := append([]storage.SettingsCheckpointItem(nil), create.Items...)
	sort.Slice(items, func(left, right int) bool {
		return settingsCheckpointItemKey(items[left]) < settingsCheckpointItemKey(items[right])
	})
	for _, item := range items {
		if err := insertSettingsCheckpointItem(ctx, tx, checkpointID, item); err != nil {
			return err
		}
	}

	return nil
}
