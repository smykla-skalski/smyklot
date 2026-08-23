package sqlstore

import (
	"context"
	"sort"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// captureInstallationSettingsSnapshot reads every persisted settings resource
// while the caller holds the installation target lock. Optional resources
// absent from this list are absent in this complete point-in-time state.
func captureInstallationSettingsSnapshot(
	ctx context.Context,
	tx *transaction,
	targetID string,
) ([]storage.SettingsCheckpointItem, error) {
	return installationSettingsBaselineItems(ctx, tx, targetID)
}

// completeInstallationSettingsCheckpoint joins two complete snapshots. Items
// absent on one side remain nil on that side, which records optional-resource
// creation and deletion without inventing a document.
func completeInstallationSettingsCheckpoint(
	before, after []storage.SettingsCheckpointItem,
) []storage.SettingsCheckpointItem {
	items := make(map[string]storage.SettingsCheckpointItem, len(before)+len(after))
	for _, captured := range before {
		item := checkpointSnapshotItem(captured)
		item.Before = cloneSettingsCheckpointState(captured.After)
		items[settingsCheckpointItemKey(item)] = item
	}
	for _, captured := range after {
		key := settingsCheckpointItemKey(captured)
		item, ok := items[key]
		if !ok {
			item = checkpointSnapshotItem(captured)
		}
		item.After = cloneSettingsCheckpointState(captured.After)
		items[key] = item
	}

	complete := make([]storage.SettingsCheckpointItem, 0, len(items))
	for _, item := range items {
		complete = append(complete, item)
	}
	sort.Slice(complete, func(left, right int) bool {
		return settingsCheckpointItemKey(complete[left]) <
			settingsCheckpointItemKey(complete[right])
	})

	return complete
}

func checkpointSnapshotItem(captured storage.SettingsCheckpointItem) storage.SettingsCheckpointItem {
	return storage.SettingsCheckpointItem{
		Kind: captured.Kind, RepositoryID: captured.RepositoryID,
		RepositoryFullName: captured.RepositoryFullName, SyncKind: captured.SyncKind,
		DocumentVersion: captured.DocumentVersion,
	}
}
