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
	incompatStateUnavailable   = "state_unavailable"
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
) (storage.SettingsCheckpointInspection, error) {
	if ref.Scope != storage.SettingsCheckpointScopeInstallation {
		return storage.SettingsCheckpointInspection{},
			errors.New("installation settings inspection needs installation scope")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("begin settings checkpoint inspection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inspection, err := inspectInstallationSettingsCheckpoint(ctx, tx, ref)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("commit settings checkpoint inspection: %w", err)
	}

	return inspection, nil
}

// InspectRootSettingsCheckpoint reads one immutable Root snapshot beside the
// current runtime singleton without turning inspection into a write.
func (s *Store) InspectRootSettingsCheckpoint(
	ctx context.Context,
	ref storage.SettingsCheckpointRef,
) (storage.SettingsCheckpointInspection, error) {
	if ref.Scope != storage.SettingsCheckpointScopeRoot || ref.TargetID != "" {
		return storage.SettingsCheckpointInspection{},
			errors.New("root settings inspection needs root scope")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("begin Root settings checkpoint inspection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inspection, err := inspectRootSettingsCheckpoint(ctx, tx, ref)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("commit Root settings checkpoint inspection: %w", err)
	}

	return inspection, nil
}

func inspectRootSettingsCheckpoint(
	ctx context.Context,
	tx *transaction,
	ref storage.SettingsCheckpointRef,
) (storage.SettingsCheckpointInspection, error) {
	checkpoint, err := getSettingsCheckpoint(ctx, tx, ref)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	current, err := getRuntimeSettings(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		current = storage.RuntimeSettings{}
	} else if err != nil {
		return storage.SettingsCheckpointInspection{},
			fmt.Errorf("read Root settings inspection state: %w", err)
	}
	currentState, err := runtimeSettingsState(runtimeSettingsDocument(current), current.Revision)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	items := make([]storage.SettingsCheckpointInspectionItem, 0, len(checkpoint.Items))
	for _, item := range checkpoint.Items {
		var currentIncompatibility *storage.SettingsCheckpointIncompatibility
		if item.Kind != storage.SettingsCheckpointItemRuntime {
			currentIncompatibility = settingsIncompatibility(
				incompatResourceGone,
				"This resource is not part of Root settings",
			)
		} else if err := validateRuntimeSettingsDocument(currentState.Document); err != nil {
			currentIncompatibility = settingsIncompatibility(
				incompatCurrentInvalid,
				"The current stored settings cannot be validated safely",
			)
		}
		items = append(items, inspectSettingsCheckpointItem(
			checkpoint.Action,
			item,
			currentState,
			currentIncompatibility,
		))
	}

	return storage.SettingsCheckpointInspection{
		Checkpoint: cloneSettingsCheckpoint(checkpoint),
		Items:      items,
	}, nil
}

func inspectInstallationSettingsCheckpoint(
	ctx context.Context,
	tx *transaction,
	ref storage.SettingsCheckpointRef,
) (storage.SettingsCheckpointInspection, error) {
	checkpoint, err := getSettingsCheckpoint(ctx, tx, ref)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	current, err := loadInstallationCheckpointCurrent(ctx, tx, ref.TargetID)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}
	items, err := inspectInstallationCheckpointItems(ctx, tx, checkpoint, current)
	if err != nil {
		return storage.SettingsCheckpointInspection{}, err
	}

	return storage.SettingsCheckpointInspection{
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
		inspected, err := inspectInstallationCheckpointItem(
			ctx, tx, checkpoint.Action, item, &current,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, inspected)
		seen[inspectionIdentityKey(inspected.Identity)] = true
	}
	withAbsences, err := appendCheckpointAbsences(
		ctx, tx, checkpoint.Action, items, seen, &current,
	)
	if err != nil {
		return nil, err
	}
	items = withAbsences
	sort.Slice(items, func(left, right int) bool {
		return inspectionIdentityKey(items[left].Identity) <
			inspectionIdentityKey(items[right].Identity)
	})

	return items, nil
}

func appendCheckpointAbsences(
	ctx context.Context,
	tx *transaction,
	action storage.SettingsCheckpointAction,
	items []storage.SettingsCheckpointInspectionItem,
	seen map[string]bool,
	current *installationCheckpointCurrent,
) ([]storage.SettingsCheckpointInspectionItem, error) {
	for kind := range current.configs {
		identity := storage.SettingsCheckpointItemIdentity{
			Kind: storage.SettingsCheckpointItemSyncConfig, SyncKind: kind,
		}
		if !seen[inspectionIdentityKey(identity)] {
			inspected, err := inspectCheckpointAbsence(
				ctx, tx, action, identity, "", current,
			)
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
			inspected, err := inspectCheckpointAbsence(
				ctx, tx, action, identity, repository.FullName, current,
			)
			if err != nil {
				return nil, err
			}
			items = append(items, inspected)
		}
	}

	return items, nil
}

func inspectCheckpointAbsence(
	ctx context.Context,
	tx *transaction,
	action storage.SettingsCheckpointAction,
	identity storage.SettingsCheckpointItemIdentity,
	fullName string,
	current *installationCheckpointCurrent,
) (storage.SettingsCheckpointInspectionItem, error) {
	item := storage.SettingsCheckpointItem{
		Kind: identity.Kind, RepositoryID: identity.RepositoryID,
		RepositoryFullName: fullName, SyncKind: identity.SyncKind,
		DocumentVersion: storage.SettingsCheckpointDocumentVersion,
	}
	inspected, err := inspectInstallationCheckpointItem(ctx, tx, action, item, current)
	if err != nil {
		return storage.SettingsCheckpointInspectionItem{}, err
	}

	return inspected, nil
}

func inspectInstallationCheckpointItem(
	ctx context.Context,
	tx *transaction,
	action storage.SettingsCheckpointAction,
	item storage.SettingsCheckpointItem,
	current *installationCheckpointCurrent,
) (storage.SettingsCheckpointInspectionItem, error) {
	state, incompatibility, err := currentInstallationCheckpointState(
		ctx, tx, item, current,
	)
	if err != nil {
		return storage.SettingsCheckpointInspectionItem{}, err
	}
	return inspectSettingsCheckpointItem(action, item, state, incompatibility), nil
}

func inspectSettingsCheckpointItem(
	action storage.SettingsCheckpointAction,
	item storage.SettingsCheckpointItem,
	current *storage.SettingsCheckpointState,
	currentIncompatibility *storage.SettingsCheckpointIncompatibility,
) storage.SettingsCheckpointInspectionItem {
	beforeAvailable := action != storage.SettingsCheckpointActionBaseline
	return storage.SettingsCheckpointInspectionItem{
		Identity: storage.SettingsCheckpointItemIdentity{
			Kind: item.Kind, RepositoryID: item.RepositoryID, SyncKind: item.SyncKind,
		},
		RepositoryFullName: item.RepositoryFullName,
		DocumentVersion:    item.DocumentVersion,
		Before: inspectSettingsCheckpointSide(
			item, beforeAvailable, item.Before, current, currentIncompatibility,
		),
		After: inspectSettingsCheckpointSide(
			item, true, item.After, current, currentIncompatibility,
		),
		Current: cloneSettingsCheckpointState(current),
		Changed: beforeAvailable && !sameSettingsCheckpointState(item.Before, item.After),
	}
}

func inspectSettingsCheckpointSide(
	item storage.SettingsCheckpointItem,
	available bool,
	state, current *storage.SettingsCheckpointState,
	currentIncompatibility *storage.SettingsCheckpointIncompatibility,
) storage.SettingsCheckpointInspectionSide {
	side := storage.SettingsCheckpointInspectionSide{
		Available: available,
		State:     cloneSettingsCheckpointState(state),
		Differs:   available && !sameSettingsCheckpointState(state, current),
	}
	if !available {
		side.Incompatibility = settingsIncompatibility(
			incompatStateUnavailable,
			"This side was not captured by the checkpoint",
		)
	} else if currentIncompatibility != nil {
		side.Incompatibility = currentIncompatibility
	} else {
		side.Incompatibility = historicalSettingsIncompatibility(item, state)
	}
	side.Restorable = side.Incompatibility == nil

	return side
}

func historicalSettingsIncompatibility(
	item storage.SettingsCheckpointItem,
	state *storage.SettingsCheckpointState,
) *storage.SettingsCheckpointIncompatibility {
	if item.DocumentVersion != storage.SettingsCheckpointDocumentVersion {
		return settingsIncompatibility(incompatUnsupportedVersion,
			"This checkpoint uses a settings format this version cannot restore")
	}
	if state == nil {
		if item.Kind == storage.SettingsCheckpointItemSyncConfig ||
			item.Kind == storage.SettingsCheckpointItemSyncOverride {
			return nil
		}
		return settingsIncompatibility(incompatSnapshotInvalid,
			"This checkpoint does not contain a restorable state for this resource")
	}
	var err error
	if item.Kind == storage.SettingsCheckpointItemRuntime {
		err = validateRuntimeSettingsDocument(state.Document)
	} else {
		err = validateInstallationSettingsDocument(item.Kind, item.SyncKind, state.Document)
	}
	if err != nil {
		return settingsIncompatibility(incompatSnapshotInvalid,
			"This checkpoint contains settings this version cannot restore")
	}

	return nil
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
