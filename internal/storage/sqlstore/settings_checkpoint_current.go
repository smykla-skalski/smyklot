package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func currentInstallationCheckpointState(
	ctx context.Context,
	tx *transaction,
	item storage.SettingsCheckpointItem,
	current *installationCheckpointCurrent,
) (*storage.SettingsCheckpointState, *storage.SettingsCheckpointIncompatibility, error) {
	switch item.Kind {
	case storage.SettingsCheckpointItemTarget:
		state, err := targetSettingsState(targetSettingsDocument(current.target), current.target.Revision)
		return validateCurrentSettingsState(item.Kind, "", state, err)
	case storage.SettingsCheckpointItemRepository:
		repository, ok, err := loadInspectionRepository(ctx, tx, current, item.RepositoryID)
		if err != nil {
			return nil, nil, err
		}
		if !ok || !repository.Available {
			return repositoryStateIfPresent(repository, ok), settingsIncompatibility(
				incompatRepositoryGone,
				"This repository is no longer available in this installation",
			), nil
		}
		state, err := repositorySettingsState(
			repositorySettingsDocument(repository), repository.Revision,
		)
		return validateCurrentSettingsState(item.Kind, "", state, err)
	case storage.SettingsCheckpointItemSyncConfig:
		value, ok := current.configs[item.SyncKind]
		if !ok {
			return nil, nil, nil
		}
		state, err := syncConfigSettingsState(syncConfigSettingsDocument(value), value.Revision)
		if err == nil {
			err = validateStoredSyncConfig(value)
		}
		return validateCurrentSettingsState(item.Kind, item.SyncKind, state, err)
	case storage.SettingsCheckpointItemSyncOverride:
		return currentSyncOverrideCheckpointState(ctx, tx, item, current)
	default:
		return nil, settingsIncompatibility(incompatResourceGone,
			"This resource is not part of installation settings"), nil
	}
}

func currentSyncOverrideCheckpointState(
	ctx context.Context,
	tx *transaction,
	item storage.SettingsCheckpointItem,
	current *installationCheckpointCurrent,
) (*storage.SettingsCheckpointState, *storage.SettingsCheckpointIncompatibility, error) {
	repository, ok, err := loadInspectionRepository(ctx, tx, current, item.RepositoryID)
	if err != nil {
		return nil, nil, err
	}
	if !ok || !repository.Available {
		return nil, settingsIncompatibility(incompatRepositoryGone,
			"This repository is no longer available in this installation"), nil
	}
	value, ok := current.overrides[syncOverrideIdentity(item.RepositoryID, item.SyncKind)]
	if !ok {
		return nil, nil, nil
	}
	state, err := syncOverrideSettingsState(syncOverrideSettingsDocument(value), value.Revision)
	if err == nil {
		err = validateCurrentSyncOverride(value)
	}

	return validateCurrentSettingsState(item.Kind, item.SyncKind, state, err)
}

func validateCurrentSettingsState(
	kind storage.SettingsCheckpointItemKind,
	syncKind orgsync.Kind,
	state *storage.SettingsCheckpointState,
	err error,
) (*storage.SettingsCheckpointState, *storage.SettingsCheckpointIncompatibility, error) {
	if err == nil && state != nil {
		err = validateInstallationSettingsDocument(kind, syncKind, state.Document)
	}
	if err != nil {
		return state, settingsIncompatibility(incompatCurrentInvalid,
			"The current stored settings cannot be validated safely"), nil
	}

	return state, nil, nil
}

func loadInspectionRepository(
	ctx context.Context,
	tx *transaction,
	current *installationCheckpointCurrent,
	repositoryID string,
) (storage.Repository, bool, error) {
	if repository, ok := current.repositories[repositoryID]; ok {
		return repository, true, nil
	}
	repository, err := getRepository(ctx, tx, current.target.ID, repositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Repository{}, false, nil
	}
	if err != nil {
		return storage.Repository{}, false, fmt.Errorf(
			"read settings inspection repository: %w", err,
		)
	}
	if repository.ID != repositoryID {
		return storage.Repository{}, false, nil
	}
	current.repositories[repositoryID] = repository

	return repository, true, nil
}

func repositoryStateIfPresent(
	repository storage.Repository,
	present bool,
) *storage.SettingsCheckpointState {
	if !present {
		return nil
	}
	state, err := repositorySettingsState(
		repositorySettingsDocument(repository), repository.Revision,
	)
	if err != nil {
		return nil
	}

	return state
}
