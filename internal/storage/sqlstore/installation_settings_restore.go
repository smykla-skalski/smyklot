package sqlstore

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const actionInstallationSettingsRestored = "installation.settings.restored"

type installationSettingsRestorePreparation struct {
	prepared preparedInstallationSettings
}

// RestoreInstallationSettings validates and applies one selected complete side
// as a new immutable settings transaction. The source checkpoint is read only.
func (s *Store) RestoreInstallationSettings(
	ctx context.Context,
	request storage.RestoreInstallationSettingsRequest,
) (storage.SaveInstallationSettingsResult, error) {
	if err := request.Validate(); err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SaveInstallationSettingsResult{},
			fmt.Errorf("begin installation settings restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockPendingCIPolicy(ctx, tx, s.dialect); err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	if err := s.lockInstallationSettingsTarget(ctx, tx, request.TargetID); err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	preparation, err := prepareInstallationSettingsRestore(ctx, tx, request)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	work, err := s.loadInstallationSettingsRestoreWork(ctx, tx, preparation.prepared)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	if len(work.items) == 0 {
		return storage.SaveInstallationSettingsResult{}, storage.ErrSettingsRestoreNoop
	}
	work.snapshotBefore, err = captureInstallationSettingsSnapshot(
		ctx, tx, request.TargetID,
	)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}

	elevation, err := s.elevatedWrite(
		ctx, tx, request.ElevationID, request.SessionTokenHash,
		request.ActorAccountID, request.TargetID, request.ChangedAt,
	)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	if err := s.applyInstallationSettings(ctx, tx, preparation.prepared.request, work); err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	checkpointID, auditEventID, err := s.recordInstallationSettingsRestore(
		ctx, tx, request, work,
	)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	if elevation != nil {
		if err := insertElevatedNotifications(
			ctx, tx, *elevation, auditEventID,
			actionInstallationSettingsRestored, request.ChangedAt,
		); err != nil {
			return storage.SaveInstallationSettingsResult{}, err
		}
	}

	result, err := s.readInstallationSettingsResult(ctx, tx, preparation.prepared)
	if err != nil {
		return storage.SaveInstallationSettingsResult{}, err
	}
	appendInstallationSettingsChanges(&result, work)
	result.CheckpointID = &checkpointID
	if err := tx.Commit(); err != nil {
		return storage.SaveInstallationSettingsResult{},
			fmt.Errorf("commit installation settings restore: %w", err)
	}

	return result, nil
}

func prepareInstallationSettingsRestore(
	ctx context.Context,
	tx *transaction,
	request storage.RestoreInstallationSettingsRequest,
) (installationSettingsRestorePreparation, error) {
	inspection, err := inspectInstallationSettingsCheckpoint(ctx, tx, storage.SettingsCheckpointRef{
		ID: request.CheckpointID, Scope: storage.SettingsCheckpointScopeInstallation,
		TargetID: request.TargetID,
	})
	if err != nil {
		return installationSettingsRestorePreparation{}, err
	}
	if inspection.Checkpoint.Action == storage.SettingsCheckpointActionBaseline &&
		request.Side == storage.SettingsCheckpointRestoreBefore {
		return installationSettingsRestorePreparation{}, fmt.Errorf(
			"%w: baseline has no before state",
			storage.ErrSettingsRestoreBlocked,
		)
	}
	batch, removals, err := installationRestoreBatch(request, inspection)
	if err != nil {
		return installationSettingsRestorePreparation{}, err
	}
	prepared, err := prepareNonRemovalRestoreBatch(batch)
	if err != nil {
		return installationSettingsRestorePreparation{}, err
	}
	appendInstallationRestoreRemovals(&prepared, removals)

	return installationSettingsRestorePreparation{prepared: prepared}, nil
}

type installationSettingsRestoreRemovals struct {
	configs   []storage.InstallationSyncConfigChange
	overrides []storage.InstallationSyncOverrideChange
}

func installationRestoreBatch(
	request storage.RestoreInstallationSettingsRequest,
	inspection storage.SettingsCheckpointInspection,
) (storage.SaveInstallationSettingsRequest, installationSettingsRestoreRemovals, error) {
	items := make(map[string]storage.SettingsCheckpointInspectionItem, len(inspection.Items))
	for _, item := range inspection.Items {
		items[inspectionIdentityKey(item.Identity)] = item
	}
	batch := storage.SaveInstallationSettingsRequest{
		TargetID: request.TargetID, ActorAccountID: request.ActorAccountID,
		ElevationID: request.ElevationID, SessionTokenHash: request.SessionTokenHash,
		ChangedAt: request.ChangedAt,
	}
	var removals installationSettingsRestoreRemovals
	selections := append([]storage.SettingsCheckpointRestoreSelection(nil), request.Selections...)
	sort.Slice(selections, func(left, right int) bool {
		return inspectionIdentityKey(selections[left].Identity) <
			inspectionIdentityKey(selections[right].Identity)
	})
	for _, selection := range selections {
		item, ok := items[inspectionIdentityKey(selection.Identity)]
		if !ok {
			return batch, removals, fmt.Errorf(
				"%w: selected resource is not represented by the source checkpoint",
				storage.ErrSettingsRestoreBlocked,
			)
		}
		if err := validateInstallationRestoreSelection(request.Side, selection, item); err != nil {
			return batch, removals, err
		}
		if err := appendInstallationRestoreSelection(
			&batch, &removals, request.Side, selection, item,
			request.DeploymentPendingCIQuietPeriod,
		); err != nil {
			return batch, removals, err
		}
	}

	return batch, removals, nil
}

func validateInstallationRestoreSelection(
	side storage.SettingsCheckpointRestoreSide,
	selection storage.SettingsCheckpointRestoreSelection,
	item storage.SettingsCheckpointInspectionItem,
) error {
	selected := settingsCheckpointInspectionSide(item, side)
	if !selected.Restorable {
		code := "incompatible"
		if selected.Incompatibility != nil {
			code = selected.Incompatibility.Code
		}

		return fmt.Errorf("%w: %s", storage.ErrSettingsRestoreBlocked, code)
	}
	currentRevision := int64(0)
	if item.Current != nil {
		currentRevision = item.Current.Revision
	}
	if currentRevision != selection.ExpectedRevision {
		return storage.ErrConflict
	}

	return nil
}

func appendInstallationRestoreSelection(
	batch *storage.SaveInstallationSettingsRequest,
	removals *installationSettingsRestoreRemovals,
	side storage.SettingsCheckpointRestoreSide,
	selection storage.SettingsCheckpointRestoreSelection,
	item storage.SettingsCheckpointInspectionItem,
	deploymentQuietPeriod time.Duration,
) error {
	state := settingsCheckpointInspectionSide(item, side).State
	switch selection.Identity.Kind {
	case storage.SettingsCheckpointItemTarget:
		if state == nil {
			return fmt.Errorf("%w: target state is absent", storage.ErrSettingsRestoreBlocked)
		}
		value, err := decodeSettingsDocument[storage.TargetSettingsDocument](state.Document)
		if err != nil {
			return blockedRestoreDecode(err)
		}
		batch.Target = &storage.InstallationTargetSettingsChange{
			RepositoryDefaultEnabled:       value.RepositoryDefaultEnabled,
			PendingCIModeDefault:           value.PendingCIModeDefault,
			PendingCIBranchPatternsDefault: value.PendingCIBranchPatternsDefault,
			PendingCIQuietPeriodOverride:   value.PendingCIQuietPeriodOverride,
			PathIndexIntervalOverride:      value.PathIndexIntervalOverride,
			ConfigPatch:                    value.ConfigPatch, ExpectedRevision: selection.ExpectedRevision,
			RetunePendingCIQuietPeriod:     true,
			DeploymentPendingCIQuietPeriod: deploymentQuietPeriod,
		}
	case storage.SettingsCheckpointItemRepository:
		if state == nil {
			return fmt.Errorf("%w: repository state is absent", storage.ErrSettingsRestoreBlocked)
		}
		value, err := decodeSettingsDocument[storage.RepositorySettingsDocument](state.Document)
		if err != nil {
			return blockedRestoreDecode(err)
		}
		batch.Repositories = append(batch.Repositories,
			storage.InstallationRepositorySettingsChange{
				RepositoryID:                    selection.Identity.RepositoryID,
				EnabledOverride:                 value.EnabledOverride,
				PendingCIModeOverride:           value.PendingCIModeOverride,
				PendingCIBranchPatternsOverride: value.PendingCIBranchPatternsOverride,
				PendingCIQuietPeriodOverride:    value.PendingCIQuietPeriodOverride,
				PathIndexIntervalOverride:       value.PathIndexIntervalOverride,
				ConfigPatch:                     value.ConfigPatch, IgnoreRepositoryFile: value.IgnoreRepositoryFile,
				ExpectedRevision:               selection.ExpectedRevision,
				RetunePendingCIQuietPeriod:     true,
				DeploymentPendingCIQuietPeriod: deploymentQuietPeriod,
			},
		)
	case storage.SettingsCheckpointItemSyncConfig:
		return appendRestoreSyncConfig(batch, removals, selection, state)
	case storage.SettingsCheckpointItemSyncOverride:
		return appendRestoreSyncOverride(batch, removals, selection, state)
	default:
		return fmt.Errorf("%w: unsupported selected resource", storage.ErrSettingsRestoreBlocked)
	}

	return nil
}

func settingsCheckpointInspectionSide(
	item storage.SettingsCheckpointInspectionItem,
	side storage.SettingsCheckpointRestoreSide,
) storage.SettingsCheckpointInspectionSide {
	if side == storage.SettingsCheckpointRestoreBefore {
		return item.Before
	}

	return item.After
}

func appendRestoreSyncConfig(
	batch *storage.SaveInstallationSettingsRequest,
	removals *installationSettingsRestoreRemovals,
	selection storage.SettingsCheckpointRestoreSelection,
	state *storage.SettingsCheckpointState,
) error {
	change := storage.InstallationSyncConfigChange{
		Kind: selection.Identity.SyncKind, ExpectedRevision: selection.ExpectedRevision,
	}
	if state == nil {
		removals.configs = append(removals.configs, change)
		return nil
	}
	value, err := decodeSettingsDocument[storage.SyncConfigSettingsDocument](state.Document)
	if err != nil {
		return blockedRestoreDecode(err)
	}
	change.Enabled = value.Enabled
	change.Document = []byte(value.Document)
	batch.SyncConfigs = append(batch.SyncConfigs, change)

	return nil
}

func appendRestoreSyncOverride(
	batch *storage.SaveInstallationSettingsRequest,
	removals *installationSettingsRestoreRemovals,
	selection storage.SettingsCheckpointRestoreSelection,
	state *storage.SettingsCheckpointState,
) error {
	change := storage.InstallationSyncOverrideChange{
		RepositoryID: selection.Identity.RepositoryID, Kind: selection.Identity.SyncKind,
		ExpectedRevision: selection.ExpectedRevision,
	}
	if state == nil {
		removals.overrides = append(removals.overrides, change)
		return nil
	}
	value, err := decodeSettingsDocument[storage.SyncOverrideSettingsDocument](state.Document)
	if err != nil {
		return blockedRestoreDecode(err)
	}
	change.Enabled = value.Enabled
	change.Document = []byte(value.Document)
	batch.SyncOverrides = append(batch.SyncOverrides, change)

	return nil
}

func blockedRestoreDecode(err error) error {
	return fmt.Errorf("%w: selected settings document cannot be decoded: %v",
		storage.ErrSettingsRestoreBlocked, err)
}

func prepareNonRemovalRestoreBatch(
	batch storage.SaveInstallationSettingsRequest,
) (preparedInstallationSettings, error) {
	if batch.Target == nil && len(batch.Repositories) == 0 &&
		len(batch.SyncConfigs) == 0 && len(batch.SyncOverrides) == 0 {
		return preparedInstallationSettings{request: batch}, nil
	}

	return prepareInstallationSettings(batch)
}

func appendInstallationRestoreRemovals(
	prepared *preparedInstallationSettings,
	removals installationSettingsRestoreRemovals,
) {
	for _, change := range removals.configs {
		prepared.syncConfigs = append(prepared.syncConfigs, preparedSyncConfigSettings{
			change: change, remove: true,
		})
	}
	for _, change := range removals.overrides {
		prepared.syncOverrides = append(prepared.syncOverrides, preparedSyncOverrideSettings{
			change: change, remove: true,
		})
	}
	sort.Slice(prepared.syncConfigs, func(left, right int) bool {
		return prepared.syncConfigs[left].change.Kind < prepared.syncConfigs[right].change.Kind
	})
	sort.Slice(prepared.syncOverrides, func(left, right int) bool {
		leftChange := prepared.syncOverrides[left].change
		rightChange := prepared.syncOverrides[right].change
		if leftChange.RepositoryID != rightChange.RepositoryID {
			return leftChange.RepositoryID < rightChange.RepositoryID
		}

		return leftChange.Kind < rightChange.Kind
	})
}

func (s *Store) loadInstallationSettingsRestoreWork(
	ctx context.Context,
	tx *transaction,
	prepared preparedInstallationSettings,
) (installationSettingsWork, error) {
	work, err := loadInstallationSettingsWork(ctx, tx, prepared)
	if err != nil {
		return installationSettingsWork{}, err
	}

	return s.loadInstallationSyncSettingsWork(ctx, tx, prepared, work)
}

func (s *Store) recordInstallationSettingsRestore(
	ctx context.Context,
	tx *transaction,
	request storage.RestoreInstallationSettingsRequest,
	work installationSettingsWork,
) (int64, int64, error) {
	after, err := captureInstallationSettingsSnapshot(ctx, tx, request.TargetID)
	if err != nil {
		return 0, 0, err
	}
	items := completeInstallationSettingsCheckpoint(work.snapshotBefore, after)
	checkpointID, err := s.createSettingsCheckpoint(ctx, tx, storage.SettingsCheckpointCreate{
		Scope: storage.SettingsCheckpointScopeInstallation, TargetID: request.TargetID,
		ActorAccountID: request.ActorAccountID, Action: storage.SettingsCheckpointActionRestore,
		RestoredFromID: &request.CheckpointID, RestoredSide: request.Side,
		CreatedAt: request.ChangedAt, Items: items,
	})
	if err != nil {
		return 0, 0, err
	}
	sourceKind := settingsCheckpointSourceKind
	summary := fmt.Sprintf("Restored %d installation settings", len(work.items))
	auditEventID, err := insertAudit(ctx, tx, auditInsert{
		TargetID: request.TargetID, SettingsCheckpointID: &checkpointID,
		ActorAccountID: request.ActorAccountID, ElevationID: request.ElevationID,
		SourceKind: &sourceKind, SourceID: &checkpointID,
		Action: actionInstallationSettingsRestored, Summary: summary, CreatedAt: request.ChangedAt,
	})
	if err != nil {
		return 0, 0, err
	}

	return checkpointID, auditEventID, nil
}
