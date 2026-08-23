package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	incompatUnsupportedVersion = "unsupported_document_version"
	incompatSnapshotInvalid    = "snapshot_incompatible"
	incompatCurrentInvalid     = "current_state_invalid"
	incompatRepositoryGone     = "repository_unavailable"
	incompatResourceGone       = "resource_unavailable"
)

type installationCheckpointCurrent struct {
	target       storage.Target
	configs      map[orgsync.Kind]orgsync.Config
	overrides    map[string]orgsync.RepositoryOverride
	repositories map[string]storage.Repository
}

// InspectInstallationSettingsCheckpoint reads immutable history together with
// the current canonical state without making restoration a write operation.
func (s *Store) InspectInstallationSettingsCheckpoint(
	ctx context.Context,
	ref storage.SettingsCheckpointRef,
) (storage.InstallationSettingsCheckpointInspection, error) {
	if ref.Scope != storage.SettingsCheckpointScopeInstallation {
		return storage.InstallationSettingsCheckpointInspection{},
			errors.New("installation settings inspection needs installation scope")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.InstallationSettingsCheckpointInspection{},
			fmt.Errorf("begin settings checkpoint inspection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inspection, err := inspectInstallationSettingsCheckpoint(ctx, tx, ref)
	if err != nil {
		return storage.InstallationSettingsCheckpointInspection{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.InstallationSettingsCheckpointInspection{},
			fmt.Errorf("commit settings checkpoint inspection: %w", err)
	}

	return inspection, nil
}

func inspectInstallationSettingsCheckpoint(
	ctx context.Context,
	tx *transaction,
	ref storage.SettingsCheckpointRef,
) (storage.InstallationSettingsCheckpointInspection, error) {
	checkpoint, err := getSettingsCheckpoint(ctx, tx, ref)
	if err != nil {
		return storage.InstallationSettingsCheckpointInspection{}, err
	}
	current, err := loadInstallationCheckpointCurrent(ctx, tx, ref.TargetID)
	if err != nil {
		return storage.InstallationSettingsCheckpointInspection{}, err
	}
	items, err := inspectInstallationCheckpointItems(ctx, tx, checkpoint, current)
	if err != nil {
		return storage.InstallationSettingsCheckpointInspection{}, err
	}

	return storage.InstallationSettingsCheckpointInspection{
		Checkpoint: cloneSettingsCheckpoint(checkpoint), Items: items,
	}, nil
}

func loadInstallationCheckpointCurrent(
	ctx context.Context,
	tx *transaction,
	targetID string,
) (installationCheckpointCurrent, error) {
	target, err := getTarget(ctx, tx, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return installationCheckpointCurrent{}, storage.ErrNotFound
	}
	if err != nil {
		return installationCheckpointCurrent{}, fmt.Errorf("read settings inspection target: %w", err)
	}
	configs, err := listSyncConfigs(ctx, tx, targetID)
	if err != nil {
		return installationCheckpointCurrent{}, err
	}
	overrides, err := listInstallationSyncOverrides(ctx, tx, targetID)
	if err != nil {
		return installationCheckpointCurrent{}, err
	}
	current := installationCheckpointCurrent{
		target: target, configs: make(map[orgsync.Kind]orgsync.Config, len(configs)),
		overrides:    make(map[string]orgsync.RepositoryOverride, len(overrides)),
		repositories: map[string]storage.Repository{},
	}
	for _, value := range configs {
		current.configs[value.Kind] = value
	}
	for _, value := range overrides {
		current.overrides[syncOverrideIdentity(value.RepositoryID, value.Kind)] = value
	}

	return current, nil
}

func listInstallationSyncOverrides(
	ctx context.Context,
	queryer runner,
	targetID string,
) ([]orgsync.RepositoryOverride, error) {
	rows, err := queryer.QueryContext(ctx, syncOverrideFrom+`
WHERE r.target_id = ?
ORDER BY o.repository_id, o.kind`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list settings inspection Sync overrides: %w", err)
	}
	values, err := collectRows(rows, scanSyncOverride)
	if err != nil {
		return nil, fmt.Errorf("read settings inspection Sync overrides: %w", err)
	}

	return values, nil
}

func inspectInstallationCheckpointItems(
	ctx context.Context,
	tx *transaction,
	checkpoint storage.SettingsCheckpoint,
	current installationCheckpointCurrent,
) ([]storage.SettingsCheckpointInspectionItem, error) {
	items := make([]storage.SettingsCheckpointInspectionItem, 0, len(checkpoint.Items))
	seen := make(map[string]bool, len(checkpoint.Items))
	for _, item := range checkpoint.Items {
		inspected, err := inspectInstallationCheckpointItem(ctx, tx, item, &current)
		if err != nil {
			return nil, err
		}
		items = append(items, inspected)
		seen[inspectionIdentityKey(inspected.Identity)] = true
	}
	if checkpoint.Action == storage.SettingsCheckpointActionBaseline {
		withAbsences, err := appendBaselineAbsences(ctx, tx, items, seen, &current)
		if err != nil {
			return nil, err
		}
		items = withAbsences
	}
	sort.Slice(items, func(left, right int) bool {
		return inspectionIdentityKey(items[left].Identity) <
			inspectionIdentityKey(items[right].Identity)
	})

	return items, nil
}

func appendBaselineAbsences(
	ctx context.Context,
	tx *transaction,
	items []storage.SettingsCheckpointInspectionItem,
	seen map[string]bool,
	current *installationCheckpointCurrent,
) ([]storage.SettingsCheckpointInspectionItem, error) {
	for kind := range current.configs {
		identity := storage.SettingsCheckpointItemIdentity{
			Kind: storage.SettingsCheckpointItemSyncConfig, SyncKind: kind,
		}
		if !seen[inspectionIdentityKey(identity)] {
			inspected, err := inspectBaselineAbsence(ctx, tx, identity, "", current)
			if err != nil {
				return nil, err
			}
			items = append(items, inspected)
		}
	}
	for _, override := range current.overrides {
		identity := storage.SettingsCheckpointItemIdentity{
			Kind:         storage.SettingsCheckpointItemSyncOverride,
			RepositoryID: override.RepositoryID, SyncKind: override.Kind,
		}
		if !seen[inspectionIdentityKey(identity)] {
			repository, _, err := loadInspectionRepository(
				ctx, tx, current, override.RepositoryID,
			)
			if err != nil {
				return nil, err
			}
			inspected, err := inspectBaselineAbsence(
				ctx, tx, identity, repository.FullName, current,
			)
			if err != nil {
				return nil, err
			}
			items = append(items, inspected)
		}
	}

	return items, nil
}

func inspectBaselineAbsence(
	ctx context.Context,
	tx *transaction,
	identity storage.SettingsCheckpointItemIdentity,
	fullName string,
	current *installationCheckpointCurrent,
) (storage.SettingsCheckpointInspectionItem, error) {
	item := storage.SettingsCheckpointItem{
		Kind: identity.Kind, RepositoryID: identity.RepositoryID,
		RepositoryFullName: fullName, SyncKind: identity.SyncKind,
		DocumentVersion: storage.SettingsCheckpointDocumentVersion,
	}
	inspected, err := inspectInstallationCheckpointItem(ctx, tx, item, current)
	if err != nil {
		return storage.SettingsCheckpointInspectionItem{}, err
	}

	return inspected, nil
}

func inspectInstallationCheckpointItem(
	ctx context.Context,
	tx *transaction,
	item storage.SettingsCheckpointItem,
	current *installationCheckpointCurrent,
) (storage.SettingsCheckpointInspectionItem, error) {
	inspected := storage.SettingsCheckpointInspectionItem{
		Identity: storage.SettingsCheckpointItemIdentity{
			Kind: item.Kind, RepositoryID: item.RepositoryID, SyncKind: item.SyncKind,
		},
		RepositoryFullName: item.RepositoryFullName,
		DocumentVersion:    item.DocumentVersion,
		Before:             cloneSettingsCheckpointState(item.Before),
		After:              cloneSettingsCheckpointState(item.After),
	}
	state, incompatibility, err := currentInstallationCheckpointState(
		ctx, tx, item, current,
	)
	if err != nil {
		return storage.SettingsCheckpointInspectionItem{}, err
	}
	inspected.Current = state
	inspected.Differs = !sameSettingsCheckpointState(item.After, state)
	if incompatibility == nil {
		incompatibility = historicalSettingsIncompatibility(item)
	}
	inspected.Incompatibility = incompatibility
	inspected.Restorable = incompatibility == nil

	return inspected, nil
}

func historicalSettingsIncompatibility(
	item storage.SettingsCheckpointItem,
) *storage.SettingsCheckpointIncompatibility {
	if item.DocumentVersion != storage.SettingsCheckpointDocumentVersion {
		return settingsIncompatibility(incompatUnsupportedVersion,
			"This checkpoint uses a settings format this version cannot restore.")
	}
	if item.After == nil {
		if item.Kind == storage.SettingsCheckpointItemSyncConfig ||
			item.Kind == storage.SettingsCheckpointItemSyncOverride {
			return nil
		}
		return settingsIncompatibility(incompatSnapshotInvalid,
			"This checkpoint does not contain a restorable state for this resource.")
	}
	if err := validateInstallationSettingsDocument(
		item.Kind, item.SyncKind, item.After.Document,
	); err != nil {
		return settingsIncompatibility(incompatSnapshotInvalid,
			"This checkpoint contains settings this version cannot restore.")
	}

	return nil
}

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
				"This repository is no longer available in this installation.",
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
		repository, ok, err := loadInspectionRepository(ctx, tx, current, item.RepositoryID)
		if err != nil {
			return nil, nil, err
		}
		if !ok || !repository.Available {
			return nil, settingsIncompatibility(incompatRepositoryGone,
				"This repository is no longer available in this installation."), nil
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
	default:
		return nil, settingsIncompatibility(incompatResourceGone,
			"This resource is not part of installation settings."), nil
	}
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
			"The current stored settings cannot be validated safely."), nil
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

func settingsIncompatibility(code, reason string) *storage.SettingsCheckpointIncompatibility {
	return &storage.SettingsCheckpointIncompatibility{Code: code, Reason: reason}
}

func sameSettingsCheckpointState(left, right *storage.SettingsCheckpointState) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Digest == right.Digest
}

func cloneSettingsCheckpoint(checkpoint storage.SettingsCheckpoint) storage.SettingsCheckpoint {
	checkpoint.Items = append([]storage.SettingsCheckpointItem(nil), checkpoint.Items...)
	for index := range checkpoint.Items {
		checkpoint.Items[index].Before = cloneSettingsCheckpointState(checkpoint.Items[index].Before)
		checkpoint.Items[index].After = cloneSettingsCheckpointState(checkpoint.Items[index].After)
	}

	return checkpoint
}

func cloneSettingsCheckpointState(
	state *storage.SettingsCheckpointState,
) *storage.SettingsCheckpointState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.Document = append([]byte(nil), state.Document...)

	return &cloned
}

func inspectionIdentityKey(identity storage.SettingsCheckpointItemIdentity) string {
	return strings.Join([]string{
		string(identity.Kind), identity.RepositoryID, string(identity.SyncKind),
	}, "\x00")
}

func syncOverrideIdentity(repositoryID string, kind orgsync.Kind) string {
	return repositoryID + "\x00" + string(kind)
}
