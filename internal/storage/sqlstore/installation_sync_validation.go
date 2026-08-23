package sqlstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func (s *Store) validateInstallationSyncDocuments(
	ctx context.Context,
	tx *transaction,
	targetID string,
	work installationSettingsWork,
) error {
	proposedFiles, err := validateInstallationSyncConfigDocuments(work.syncConfigs)
	if err != nil {
		return err
	}
	var currentFiles *orgsync.Config
	if proposedFiles == nil && hasInstallationFilesOverride(work.syncOverrides) {
		stored, readErr := syncConfigForUpdate(
			ctx, tx, s.dialect, targetID, orgsync.KindFiles,
		)
		switch {
		case errors.Is(readErr, storage.ErrNotFound):
		case readErr != nil:
			return readErr
		default:
			currentFiles = &stored
		}
	}

	return validateInstallationSyncOverrideDocuments(
		work.syncOverrides, proposedFiles, currentFiles,
	)
}

func validateInstallationSyncConfigDocuments(
	configs []syncConfigSettingsWork,
) (*orgsync.FileConfig, error) {
	var proposedFiles *orgsync.FileConfig
	for _, config := range configs {
		if config.prepared.remove {
			if config.current != nil {
				if err := validateRestorableSyncConfig(
					config.current.Kind, syncConfigSettingsDocument(*config.current),
				); err != nil {
					return nil, err
				}
			}
			if config.prepared.change.Kind == orgsync.KindFiles {
				proposedFiles = &orgsync.FileConfig{}
			}
			continue
		}
		document := config.prepared.document
		var err error
		switch config.prepared.change.Kind {
		case orgsync.KindLabels:
			_, err = validateInstallationSyncDocument[orgsync.LabelConfig](document)
		case orgsync.KindSettings:
			_, err = validateInstallationSyncDocument[orgsync.SettingsConfig](document)
		case orgsync.KindRulesets:
			_, err = validateInstallationSyncDocument[orgsync.RulesetConfig](document)
		case orgsync.KindFiles:
			var files orgsync.FileConfig
			files, err = validateInstallationSyncDocument[orgsync.FileConfig](document)
			if err == nil {
				proposedFiles = &files
			}
		default:
			err = fmt.Errorf("%w: unknown sync kind %q",
				storage.ErrNotFound, config.prepared.change.Kind)
		}
		if err != nil {
			return nil, fmt.Errorf(
				"validate %s sync config: %w", config.prepared.change.Kind, err,
			)
		}
	}

	return proposedFiles, nil
}

func validateInstallationSyncDocument[Document interface{ Validate() error }](
	document []byte,
) (Document, error) {
	var decoded Document
	if err := requireInstallationSyncObject(document); err != nil {
		return decoded, err
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return decoded, fmt.Errorf("%w: %w", orgsync.ErrInvalidConfig, err)
	}
	if err := decoded.Validate(); err != nil {
		return decoded, err
	}

	return decoded, nil
}

func requireInstallationSyncObject(document []byte) error {
	trimmed := bytes.TrimSpace(document)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("%w: sync document must be a JSON object", orgsync.ErrInvalidConfig)
	}

	return nil
}

func validateInstallationSyncOverrideDocuments(
	overrides []syncOverrideSettingsWork,
	proposedFiles *orgsync.FileConfig,
	currentFiles *orgsync.Config,
) error {
	for _, override := range overrides {
		kind := override.prepared.change.Kind
		if err := validateCurrentInstallationSyncOverride(override); err != nil {
			return fmt.Errorf("validate stored %s sync override for %s: %w",
				kind, override.repository.ID, err)
		}
		if override.prepared.remove {
			continue
		}
		if kind != orgsync.KindFiles {
			if !bytes.Equal(override.prepared.document, []byte(emptyDocument)) {
				return fmt.Errorf(
					"%w: a %s sync override cannot carry a document",
					orgsync.ErrInvalidConfig, kind,
				)
			}
			continue
		}
		if err := validateInstallationFilesOverride(
			override, proposedFiles, currentFiles,
		); err != nil {
			return fmt.Errorf("validate files sync override for %s: %w",
				override.repository.ID, err)
		}
	}

	return nil
}

// The current row was read under the transaction's row lock. A dirty write may
// only capture and replace it after proving that its before state still has a
// document this version understands. Exact no-ops do not overwrite anything.
func validateCurrentInstallationSyncOverride(override syncOverrideSettingsWork) error {
	if override.current == nil ||
		(!override.prepared.remove && sameSyncOverride(*override.current, override.prepared)) {
		return nil
	}
	if override.current.Kind != orgsync.KindFiles {
		return validateInstallationEmptyOverride(override.current.Document)
	}

	current, err := decodeInstallationFilesOverride(override.current.Document)
	if err != nil {
		return err
	}
	// Every current path is kept so validation covers only facts intrinsic to
	// the document. Template membership may have changed since this row landed.
	return current.ValidateAgainst(orgsync.FileConfig{}, current.Adjusted())
}

func validateInstallationEmptyOverride(document []byte) error {
	if err := requireInstallationSyncObject(document); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	var empty struct{}
	if err := decoder.Decode(&empty); err != nil {
		return fmt.Errorf("%w: stored override is not an empty object: %w",
			orgsync.ErrInvalidConfig, err)
	}

	return nil
}

func validateInstallationFilesOverride(
	override syncOverrideSettingsWork,
	proposedFiles *orgsync.FileConfig,
	currentFiles *orgsync.Config,
) error {
	adjustments, err := decodeInstallationFilesOverride(override.prepared.document)
	if err != nil {
		return err
	}
	keeping := installationFilesAlreadyAdjusted(override.current)
	files := orgsync.FileConfig{}
	if proposedFiles != nil {
		files = *proposedFiles
	} else if installationFilesAdjustBeyond(adjustments, keeping) && currentFiles != nil {
		if err := validateStoredSyncConfig(*currentFiles); err != nil {
			return fmt.Errorf("the stored files sync config failed integrity validation: %w", err)
		}
		if err := json.Unmarshal(currentFiles.Document, &files); err != nil {
			return fmt.Errorf("%w: the stored files sync config cannot be read: %w",
				orgsync.ErrInvalidConfig, err)
		}
		if err := files.Validate(); err != nil {
			return err
		}
	}

	return adjustments.ValidateAgainst(files, keeping)
}

func decodeInstallationFilesOverride(document []byte) (orgsync.FileOverride, error) {
	if err := requireInstallationSyncObject(document); err != nil {
		return orgsync.FileOverride{}, err
	}
	var adjustments orgsync.FileOverride
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&adjustments); err != nil {
		return orgsync.FileOverride{}, fmt.Errorf("%w: %w", orgsync.ErrInvalidConfig, err)
	}

	return adjustments, nil
}

func hasInstallationFilesOverride(overrides []syncOverrideSettingsWork) bool {
	for _, override := range overrides {
		if override.prepared.change.Kind == orgsync.KindFiles {
			return true
		}
	}

	return false
}

func installationFilesAlreadyAdjusted(current *orgsync.RepositoryOverride) []string {
	if current == nil {
		return nil
	}
	saved, err := decodeInstallationFilesOverride(current.Document)
	if err != nil {
		return nil
	}

	return saved.Adjusted()
}

func installationFilesAdjustBeyond(adjustments orgsync.FileOverride, keeping []string) bool {
	held := make(map[string]bool, len(keeping))
	for _, path := range keeping {
		held[path] = true
	}
	for _, path := range adjustments.Adjusted() {
		if !held[path] {
			return true
		}
	}

	return false
}
