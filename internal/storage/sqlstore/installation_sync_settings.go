package sqlstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type preparedSyncConfigSettings struct {
	change   storage.InstallationSyncConfigChange
	document []byte
	digest   string
	remove   bool
}

type preparedSyncOverrideSettings struct {
	change   storage.InstallationSyncOverrideChange
	document []byte
	remove   bool
}

type syncConfigSettingsWork struct {
	prepared      preparedSyncConfigSettings
	current       *orgsync.Config
	afterRevision int64
	changed       bool
}

type syncOverrideSettingsWork struct {
	prepared      preparedSyncOverrideSettings
	repository    storage.Repository
	current       *orgsync.RepositoryOverride
	afterRevision int64
	changed       bool
}

func prepareInstallationSyncSettings(prepared *preparedInstallationSettings) error {
	configs, err := prepareInstallationSyncConfigs(prepared.request.SyncConfigs)
	if err != nil {
		return err
	}
	overrides, err := prepareInstallationSyncOverrides(prepared.request.SyncOverrides)
	if err != nil {
		return err
	}
	prepared.syncConfigs = configs
	prepared.syncOverrides = overrides

	return nil
}

func prepareInstallationSyncConfigs(
	changes []storage.InstallationSyncConfigChange,
) ([]preparedSyncConfigSettings, error) {
	prepared := make([]preparedSyncConfigSettings, 0, len(changes))
	for _, change := range changes {
		if !change.Kind.Valid() {
			return nil, fmt.Errorf("%w: unknown sync kind %q", storage.ErrNotFound, change.Kind)
		}
		if change.ExpectedRevision < 0 {
			return nil, errors.New("sync config revision cannot be negative")
		}
		if !json.Valid(change.Document) {
			return nil, fmt.Errorf("%s sync config document must be valid JSON", change.Kind)
		}
		document := append([]byte(nil), change.Document...)
		prepared = append(prepared, preparedSyncConfigSettings{
			change: change, document: document,
			digest: orgsync.DigestConfig(change.Enabled, document),
		})
	}
	sort.Slice(prepared, func(left, right int) bool {
		return prepared[left].change.Kind < prepared[right].change.Kind
	})
	for index := 1; index < len(prepared); index++ {
		if prepared[index-1].change.Kind == prepared[index].change.Kind {
			return nil, fmt.Errorf("duplicate sync config kind %q", prepared[index].change.Kind)
		}
	}

	return prepared, nil
}

func prepareInstallationSyncOverrides(
	changes []storage.InstallationSyncOverrideChange,
) ([]preparedSyncOverrideSettings, error) {
	prepared := make([]preparedSyncOverrideSettings, 0, len(changes))
	for _, change := range changes {
		if strings.TrimSpace(change.RepositoryID) == "" {
			return nil, errors.New("sync override repository is required")
		}
		if !change.Kind.Valid() {
			return nil, fmt.Errorf("%w: unknown sync kind %q", storage.ErrNotFound, change.Kind)
		}
		if change.ExpectedRevision < 0 {
			return nil, errors.New("sync override revision cannot be negative")
		}
		document := []byte(syncDocumentColumn(change.Document))
		if !json.Valid(document) {
			return nil, fmt.Errorf("%s sync override document must be valid JSON", change.Kind)
		}
		change.Enabled = cloneOptionalBool(change.Enabled)
		prepared = append(prepared, preparedSyncOverrideSettings{
			change: change, document: append([]byte(nil), document...),
		})
	}
	sort.Slice(prepared, func(left, right int) bool {
		leftChange := prepared[left].change
		rightChange := prepared[right].change
		if leftChange.RepositoryID != rightChange.RepositoryID {
			return leftChange.RepositoryID < rightChange.RepositoryID
		}
		return leftChange.Kind < rightChange.Kind
	})
	for index := 1; index < len(prepared); index++ {
		previous := prepared[index-1].change
		current := prepared[index].change
		if previous.RepositoryID == current.RepositoryID && previous.Kind == current.Kind {
			return nil, fmt.Errorf(
				"duplicate sync override %q %q", current.RepositoryID, current.Kind,
			)
		}
	}

	return prepared, nil
}

func cloneOptionalBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value

	return &cloned
}

func (s *Store) loadInstallationSyncSettingsWork(
	ctx context.Context,
	tx *transaction,
	prepared preparedInstallationSettings,
	work installationSettingsWork,
) (installationSettingsWork, error) {
	work, err := s.loadInstallationSyncConfigWork(ctx, tx, prepared, work)
	if err != nil {
		return installationSettingsWork{}, err
	}
	work, err = s.loadInstallationSyncOverrideWork(ctx, tx, prepared, work)
	if err != nil {
		return installationSettingsWork{}, err
	}
	if err := validateInstallationSyncRevisions(work); err != nil {
		return installationSettingsWork{}, err
	}
	if err := s.validateInstallationSyncDocuments(
		ctx, tx, prepared.request.TargetID, work,
	); err != nil {
		return installationSettingsWork{}, err
	}

	return s.buildInstallationSyncItems(ctx, tx, prepared.request.TargetID, work)
}

func (s *Store) loadInstallationSyncConfigWork(
	ctx context.Context,
	tx *transaction,
	prepared preparedInstallationSettings,
	work installationSettingsWork,
) (installationSettingsWork, error) {
	for _, config := range prepared.syncConfigs {
		current, err := syncConfigForUpdate(
			ctx, tx, s.dialect, prepared.request.TargetID, config.change.Kind,
		)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return installationSettingsWork{}, err
		}
		entry := syncConfigSettingsWork{prepared: config}
		if err == nil {
			if err := validateStoredSyncConfig(current); err != nil {
				return installationSettingsWork{}, err
			}
			entry.current = &current
		}
		work.syncConfigs = append(work.syncConfigs, entry)
	}

	return work, nil
}

func (s *Store) loadInstallationSyncOverrideWork(
	ctx context.Context,
	tx *transaction,
	prepared preparedInstallationSettings,
	work installationSettingsWork,
) (installationSettingsWork, error) {
	repositories := map[string]storage.Repository{}
	for _, override := range prepared.syncOverrides {
		repository, err := loadSyncOverrideRepository(
			ctx, tx, prepared.request.TargetID, override.change.RepositoryID, repositories,
		)
		if err != nil {
			return installationSettingsWork{}, err
		}
		current, err := syncOverrideForUpdate(
			ctx, tx, s.dialect, repository.ID, override.change.Kind,
		)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return installationSettingsWork{}, err
		}
		entry := syncOverrideSettingsWork{prepared: override, repository: repository}
		if err == nil {
			if !json.Valid(current.Document) {
				return installationSettingsWork{}, fmt.Errorf(
					"%w: stored %s sync override document must be valid JSON",
					orgsync.ErrInvalidConfig, current.Kind,
				)
			}
			entry.current = &current
		}
		work.syncOverrides = append(work.syncOverrides, entry)
	}

	return work, nil
}

func loadSyncOverrideRepository(
	ctx context.Context,
	tx *transaction,
	targetID, repositoryID string,
	held map[string]storage.Repository,
) (storage.Repository, error) {
	if repository, ok := held[repositoryID]; ok {
		return repository, nil
	}
	repository, err := getRepository(ctx, tx, targetID, repositoryID)
	if err != nil {
		return storage.Repository{}, fmt.Errorf("read sync override repository: %w", noRows(err))
	}
	if repository.ID != repositoryID {
		return storage.Repository{}, storage.ErrNotFound
	}
	held[repositoryID] = repository

	return repository, nil
}

func validateStoredSyncConfig(config orgsync.Config) error {
	if !json.Valid(config.Document) {
		return fmt.Errorf("stored %s sync config document must be valid JSON", config.Kind)
	}
	if orgsync.DigestConfig(config.Enabled, config.Document) != config.Digest {
		return fmt.Errorf("stored %s sync config digest does not match", config.Kind)
	}

	return nil
}

func validateInstallationSyncRevisions(work installationSettingsWork) error {
	for _, config := range work.syncConfigs {
		if (config.current == nil && config.prepared.change.ExpectedRevision != 0) ||
			(config.current != nil &&
				config.current.Revision != config.prepared.change.ExpectedRevision) {
			return storage.ErrConflict
		}
	}
	for _, override := range work.syncOverrides {
		if (override.current == nil && override.prepared.change.ExpectedRevision != 0) ||
			(override.current != nil &&
				override.current.Revision != override.prepared.change.ExpectedRevision) {
			return storage.ErrConflict
		}
	}

	return nil
}

func syncOverrideForUpdate(
	ctx context.Context,
	tx *transaction,
	dialect Dialect,
	repositoryID string,
	kind orgsync.Kind,
) (orgsync.RepositoryOverride, error) {
	override, err := scanSyncOverride(tx.QueryRowContext(ctx, `
SELECT repository_id, kind, enabled_override, document, revision, updated_by, updated_at
FROM sync_repository_overrides
WHERE repository_id = ? AND kind = ?`+dialect.RowLock(), repositoryID, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return orgsync.RepositoryOverride{}, storage.ErrNotFound
	}
	if err != nil {
		return orgsync.RepositoryOverride{}, fmt.Errorf("read sync override for update: %w", err)
	}

	return override, nil
}

func sameSyncOverride(
	current orgsync.RepositoryOverride,
	prepared preparedSyncOverrideSettings,
) bool {
	return sameOptionalBool(current.Enabled, prepared.change.Enabled) &&
		bytes.Equal(current.Document, prepared.document)
}
