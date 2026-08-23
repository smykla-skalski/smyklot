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

func validateInstallationFilesOverride(
	override syncOverrideSettingsWork,
	proposedFiles *orgsync.FileConfig,
	currentFiles *orgsync.Config,
) error {
	if err := requireInstallationSyncObject(override.prepared.document); err != nil {
		return err
	}
	var adjustments orgsync.FileOverride
	decoder := json.NewDecoder(bytes.NewReader(override.prepared.document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&adjustments); err != nil {
		return fmt.Errorf("%w: %w", orgsync.ErrInvalidConfig, err)
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
	var saved orgsync.FileOverride
	if err := json.Unmarshal(current.Document, &saved); err != nil {
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
