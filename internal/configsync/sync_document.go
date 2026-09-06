package configsync

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// Check typed field names before any semantic merge can reorder their keys.
// Arbitrary JSON inside template adjustments is retained by RawMessage fields.
func validateFileSyncShapes(section *config.PanelFileSection) error {
	if err := section.Validate(); err != nil {
		return err
	}
	for kind, item := range section.Sync {
		var destination any
		if section.Scope == config.PanelFileRepository {
			destination = &struct{}{}
			if orgsync.Kind(kind) == orgsync.KindFiles {
				destination = &orgsync.FileOverride{}
			}
		} else {
			switch orgsync.Kind(kind) {
			case orgsync.KindFiles:
				destination = &orgsync.FileConfig{}
			case orgsync.KindSettings:
				destination = &orgsync.SettingsConfig{}
			case orgsync.KindLabels:
				destination = &orgsync.LabelConfig{}
			case orgsync.KindRulesets:
				destination = &orgsync.RulesetConfig{}
			}
		}
		if err := config.DecodeExactJSON(item.Document, destination); err != nil {
			return fmt.Errorf("sync %s document: %w", kind, err)
		}
	}
	return nil
}

// Keep authored Sync bytes when only their JSON presentation differs. Their
// stored digest includes those bytes, so reserializing an unchanged document
// would create a revision and invalidate pending sync work for no setting change.
func preservedSyncDocument(snapshot PanelSnapshot, kind orgsync.Kind, proposed []byte, exists bool) ([]byte, error) {
	if !exists {
		return nil, nil
	}
	var current []byte
	if snapshot.Repository == nil {
		for _, item := range snapshot.SyncConfigs {
			if item.Kind == kind {
				current = item.Document
			}
		}
	} else {
		for _, item := range snapshot.SyncOverrides {
			if item.Kind == kind {
				current = item.Document
			}
		}
	}
	if len(bytes.TrimSpace(current)) != 0 {
		same, err := Equivalent(current, proposed)
		if err != nil {
			return nil, err
		}
		if same {
			return slices.Clone(current), nil
		}
	}
	return slices.Clone(proposed), nil
}
