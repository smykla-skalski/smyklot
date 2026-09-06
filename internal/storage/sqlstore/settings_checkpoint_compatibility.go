package sqlstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func validateInstallationSettingsDocument(
	kind storage.SettingsCheckpointItemKind,
	syncKind orgsync.Kind,
	document []byte,
) error {
	switch kind {
	case storage.SettingsCheckpointItemTarget:
		value, err := decodeSettingsDocument[storage.TargetSettingsDocument](document)
		if err != nil {
			return err
		}
		return validateRestorableTargetDocument(value)
	case storage.SettingsCheckpointItemRepository:
		value, err := decodeSettingsDocument[storage.RepositorySettingsDocument](document)
		if err != nil {
			return err
		}
		return validateRestorableRepositoryDocument(value)
	case storage.SettingsCheckpointItemSyncConfig:
		value, err := decodeSettingsDocument[storage.SyncConfigSettingsDocument](document)
		if err != nil {
			return err
		}
		return validateRestorableSyncConfig(syncKind, value)
	case storage.SettingsCheckpointItemSyncOverride:
		value, err := decodeSettingsDocument[storage.SyncOverrideSettingsDocument](document)
		if err != nil {
			return err
		}
		return validateRestorableSyncOverride(syncKind, value)
	default:
		return fmt.Errorf("unsupported settings document kind %q", kind)
	}
}

func validateRuntimeSettingsDocument(document []byte) error {
	value, err := decodeSettingsDocument[storage.RuntimeSettingsDocument](document)
	if err != nil {
		return err
	}

	return validateRuntimeSettingsDocumentValue(value)
}

func validateRuntimeSettingsDocumentValue(value storage.RuntimeSettingsDocument) error {
	if value.LogLevel != nil && !validRuntimeLogLevel(*value.LogLevel) {
		return fmt.Errorf("unsupported runtime log level %q", *value.LogLevel)
	}
	if value.BotConfig != nil {
		if _, err := config.ParseRunner(string(value.BotConfig.Runner)); err != nil {
			return fmt.Errorf("invalid runtime behavior defaults: %w", err)
		}
	}
	if err := validateRuntimeRestoreDuration(
		value.PollInterval,
		storage.MinRuntimePollInterval,
		storage.MaxRuntimePollInterval,
		true,
		"reaction sweep interval",
	); err != nil {
		return err
	}
	if err := validateRuntimeRestoreDuration(
		value.PendingCIQuietPeriod,
		pendingci.MinPassingQuiet,
		pendingci.MaxPassingQuiet,
		false,
		"merge-after-CI quiet period",
	); err != nil {
		return err
	}
	if err := validateRuntimeRestoreDuration(
		value.SessionTTL,
		time.Minute,
		storage.MaxRuntimeSessionTTL,
		false,
		"session lifetime",
	); err != nil {
		return err
	}
	return validateRuntimeRestoreDuration(
		value.PathIndexInterval,
		0,
		storage.MaxPathIndexInterval,
		false,
		"file list refresh interval",
	)
}

func validateRuntimeRestoreDuration(
	value *time.Duration,
	minimum, maximum time.Duration,
	allowDisabled bool,
	label string,
) error {
	if value == nil {
		return nil
	}
	if *value%time.Second != 0 || *value < 0 || *value > maximum {
		return fmt.Errorf("%s is outside the supported range", label)
	}
	if *value == 0 && allowDisabled {
		return nil
	}
	if *value < minimum {
		return fmt.Errorf("%s is below the supported range", label)
	}

	return nil
}

func decodeSettingsDocument[T any](document []byte) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return value, errors.New("settings document contains trailing JSON")
	}

	return value, nil
}

func validateRestorableTargetDocument(value storage.TargetSettingsDocument) error {
	if err := value.PendingCIBypassPolicyDefault.Validate(); err != nil {
		return err
	}
	if err := storage.ValidateTargetPendingCISettings(
		value.PendingCIModeDefault, value.PendingCIBranchPatternsDefault,
		value.PendingCIQuietPeriodOverride,
	); err != nil {
		return err
	}
	if err := validateRestorablePathIndex(value.PathIndexIntervalOverride); err != nil {
		return err
	}

	return validateRestorablePatch(value.ConfigPatch)
}

func validateRestorableRepositoryDocument(value storage.RepositorySettingsDocument) error {
	if err := value.PendingCIBypassPolicyOverride.Validate(); err != nil {
		return err
	}
	if err := storage.ValidateRepositoryPendingCISettings(
		value.PendingCIModeOverride, value.PendingCIBranchPatternsOverride,
		value.PendingCIQuietPeriodOverride,
	); err != nil {
		return err
	}
	if err := validateRestorablePathIndex(value.PathIndexIntervalOverride); err != nil {
		return err
	}

	return validateRestorablePatch(value.ConfigPatch)
}

func validateRestorablePathIndex(value *time.Duration) error {
	if value != nil && (*value < 0 || *value > storage.MaxPathIndexInterval ||
		*value%time.Second != 0) {
		return errors.New("file list refresh interval is out of range")
	}

	return nil
}

func validateRestorablePatch(patch config.Patch) error {
	return storage.ValidatePanelPatch(patch)
}

func validateRestorableSyncConfig(
	kind orgsync.Kind,
	value storage.SyncConfigSettingsDocument,
) error {
	if !json.Valid([]byte(value.Document)) {
		return errors.New("sync configuration document is not valid JSON")
	}
	switch kind {
	case orgsync.KindLabels:
		_, err := validateInstallationSyncDocument[orgsync.LabelConfig]([]byte(value.Document))
		return err
	case orgsync.KindSettings:
		_, err := validateInstallationSyncDocument[orgsync.SettingsConfig]([]byte(value.Document))
		return err
	case orgsync.KindRulesets:
		_, err := validateInstallationSyncDocument[orgsync.RulesetConfig]([]byte(value.Document))
		return err
	case orgsync.KindFiles:
		_, err := validateInstallationSyncDocument[orgsync.FileConfig]([]byte(value.Document))
		return err
	default:
		return errors.New("sync configuration kind is not supported")
	}
}

func validateRestorableSyncOverride(
	kind orgsync.Kind,
	value storage.SyncOverrideSettingsDocument,
) error {
	if !json.Valid([]byte(value.Document)) {
		return errors.New("sync override document is not valid JSON")
	}
	if kind != orgsync.KindFiles {
		return validateInstallationEmptyOverride([]byte(value.Document))
	}
	decoded, err := decodeInstallationFilesOverride([]byte(value.Document))
	if err != nil {
		return err
	}

	return decoded.ValidateAgainst(orgsync.FileConfig{}, decoded.Adjusted())
}

func validateCurrentSyncOverride(value orgsync.RepositoryOverride) error {
	work := syncOverrideSettingsWork{
		current: &value,
		prepared: preparedSyncOverrideSettings{change: storage.InstallationSyncOverrideChange{
			Kind: value.Kind, Enabled: cloneOptionalBool(value.Enabled),
		}, document: append([]byte(nil), value.Document...), remove: true},
	}

	return validateCurrentInstallationSyncOverride(work)
}
