package sqlstore

import (
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func rootRuntimeRestoreItem(
	inspection storage.SettingsCheckpointInspection,
	side storage.SettingsCheckpointRestoreSide,
) (storage.SettingsCheckpointInspectionItem, storage.SettingsCheckpointInspectionSide, error) {
	for _, item := range inspection.Items {
		if item.Identity.Kind != storage.SettingsCheckpointItemRuntime {
			continue
		}
		selected := settingsCheckpointInspectionSide(item, side)
		if !selected.Restorable || selected.State == nil {
			return storage.SettingsCheckpointInspectionItem{},
				storage.SettingsCheckpointInspectionSide{}, fmt.Errorf(
					"%w: runtime checkpoint is incompatible",
					storage.ErrSettingsRestoreBlocked,
				)
		}
		return item, selected, nil
	}

	return storage.SettingsCheckpointInspectionItem{},
		storage.SettingsCheckpointInspectionSide{}, fmt.Errorf(
			"%w: runtime checkpoint item is missing",
			storage.ErrSettingsRestoreBlocked,
		)
}
