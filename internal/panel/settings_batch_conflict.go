package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const (
	workspaceSettingsResourceTarget       = "target"
	workspaceSettingsResourceRepository   = "repository"
	workspaceSettingsResourceSyncConfig   = "sync_config"
	workspaceSettingsResourceSyncOverride = "sync_override"
)

type workspaceSettingsBatchConflictResponse struct {
	Error workspaceSettingsBatchConflictError `json:"error"`
}

type workspaceSettingsBatchConflictError struct {
	Code      string                           `json:"code"`
	Message   string                           `json:"message"`
	Conflicts []workspaceSettingsBatchConflict `json:"conflicts"`
}

type workspaceSettingsBatchConflict struct {
	Resource         string          `json:"resource"`
	TargetID         string          `json:"target_id"`
	RepositoryID     string          `json:"repository_id,omitempty"`
	Kind             orgsync.Kind    `json:"kind,omitempty"`
	ExpectedRevision int64           `json:"expected_revision"`
	ActualRevision   int64           `json:"actual_revision"`
	Latest           json.RawMessage `json:"latest,omitempty"`
}

func (s *Server) writeWorkspaceSettingsBatchConflict(
	w http.ResponseWriter,
	ctx context.Context,
	request storage.SaveInstallationSettingsRequest,
) bool {
	conflicts, err := s.workspaceSettingsBatchConflicts(ctx, request)
	if err != nil || len(conflicts) == 0 {
		return false
	}
	writeJSON(w, http.StatusConflict, workspaceSettingsBatchConflictResponse{
		Error: workspaceSettingsBatchConflictError{
			Code:      "conflict",
			Message:   "settings changed in another session; review the latest values",
			Conflicts: conflicts,
		},
	})

	return true
}

func (s *Server) workspaceSettingsBatchConflicts(
	ctx context.Context,
	request storage.SaveInstallationSettingsRequest,
) ([]workspaceSettingsBatchConflict, error) {
	conflicts := make([]workspaceSettingsBatchConflict, 0)
	if request.Target != nil {
		conflict, stale, err := s.targetSettingsBatchConflict(ctx, request.TargetID, *request.Target)
		if err != nil {
			return nil, err
		}
		if stale {
			conflicts = append(conflicts, conflict)
		}
	}
	for _, change := range request.Repositories {
		conflict, stale, err := s.repositorySettingsBatchConflict(ctx, request.TargetID, change)
		if err != nil {
			return nil, err
		}
		if stale {
			conflicts = append(conflicts, conflict)
		}
	}
	for _, change := range request.SyncConfigs {
		conflict, stale, err := s.syncConfigSettingsBatchConflict(ctx, request.TargetID, change)
		if err != nil {
			return nil, err
		}
		if stale {
			conflicts = append(conflicts, conflict)
		}
	}
	for _, change := range request.SyncOverrides {
		conflict, stale, err := s.syncOverrideSettingsBatchConflict(ctx, request.TargetID, change)
		if err != nil {
			return nil, err
		}
		if stale {
			conflicts = append(conflicts, conflict)
		}
	}
	sort.Slice(conflicts, func(left, right int) bool {
		return workspaceSettingsBatchConflictKey(conflicts[left]) <
			workspaceSettingsBatchConflictKey(conflicts[right])
	})

	return conflicts, nil
}

func workspaceSettingsBatchConflictKey(conflict workspaceSettingsBatchConflict) string {
	order := map[string]string{
		workspaceSettingsResourceTarget:       "0",
		workspaceSettingsResourceRepository:   "1",
		workspaceSettingsResourceSyncConfig:   "2",
		workspaceSettingsResourceSyncOverride: "3",
	}

	return order[conflict.Resource] + "\x00" + conflict.RepositoryID + "\x00" + string(conflict.Kind)
}

func (s *Server) targetSettingsBatchConflict(
	ctx context.Context,
	targetID string,
	change storage.InstallationTargetSettingsChange,
) (workspaceSettingsBatchConflict, bool, error) {
	conflict := workspaceSettingsBatchConflict{
		Resource: workspaceSettingsResourceTarget,
		TargetID: targetID, ExpectedRevision: change.ExpectedRevision,
	}
	target, err := s.store.GetTarget(ctx, targetID)
	if errors.Is(err, storage.ErrNotFound) {
		return conflict, change.ExpectedRevision != 0, nil
	}
	if err != nil {
		return conflict, false, err
	}
	conflict.ActualRevision = target.Revision
	if target.Revision == change.ExpectedRevision {
		return conflict, false, nil
	}
	conflict.Latest, _ = json.Marshal(workspaceTargetSettingsStateFrom(target))

	return conflict, true, nil
}

func (s *Server) repositorySettingsBatchConflict(
	ctx context.Context,
	targetID string,
	change storage.InstallationRepositorySettingsChange,
) (workspaceSettingsBatchConflict, bool, error) {
	conflict := workspaceSettingsBatchConflict{
		Resource: workspaceSettingsResourceRepository,
		TargetID: targetID, RepositoryID: change.RepositoryID,
		ExpectedRevision: change.ExpectedRevision,
	}
	repository, err := s.store.GetRepository(ctx, targetID, change.RepositoryID)
	if errors.Is(err, storage.ErrNotFound) {
		return conflict, change.ExpectedRevision != 0, nil
	}
	if err != nil {
		return conflict, false, err
	}
	conflict.ActualRevision = repository.Revision
	if repository.Revision == change.ExpectedRevision {
		return conflict, false, nil
	}
	conflict.Latest, _ = json.Marshal(workspaceRepositorySettingsStateFrom(repository))

	return conflict, true, nil
}

func (s *Server) syncConfigSettingsBatchConflict(
	ctx context.Context,
	targetID string,
	change storage.InstallationSyncConfigChange,
) (workspaceSettingsBatchConflict, bool, error) {
	conflict := workspaceSettingsBatchConflict{
		Resource: workspaceSettingsResourceSyncConfig,
		TargetID: targetID, Kind: change.Kind,
		ExpectedRevision: change.ExpectedRevision,
	}
	config, err := s.store.GetSyncConfig(ctx, targetID, change.Kind)
	if errors.Is(err, storage.ErrNotFound) {
		if change.ExpectedRevision == 0 {
			return conflict, false, nil
		}
		conflict.Latest, _ = json.Marshal(workspaceSyncConfigSettingsState{
			TargetID: targetID, Kind: change.Kind, Document: emptyDocument,
		})
		return conflict, true, nil
	}
	if err != nil {
		return conflict, false, err
	}
	conflict.ActualRevision = config.Revision
	if config.Revision == change.ExpectedRevision {
		return conflict, false, nil
	}
	if _, validationErr := validatedSyncDocument(config.Kind, config.Document); validationErr == nil {
		conflict.Latest, _ = json.Marshal(workspaceSyncConfigSettingsState{
			TargetID: targetID, Kind: config.Kind, Enabled: config.Enabled,
			Document: config.Document, Revision: config.Revision,
		})
	}

	return conflict, true, nil
}

func (s *Server) syncOverrideSettingsBatchConflict(
	ctx context.Context,
	targetID string,
	change storage.InstallationSyncOverrideChange,
) (workspaceSettingsBatchConflict, bool, error) {
	conflict := workspaceSettingsBatchConflict{
		Resource: workspaceSettingsResourceSyncOverride,
		TargetID: targetID, RepositoryID: change.RepositoryID,
		Kind: change.Kind, ExpectedRevision: change.ExpectedRevision,
	}
	override, err := s.store.GetSyncRepositoryOverride(
		ctx, targetID, change.RepositoryID, change.Kind,
	)
	if errors.Is(err, storage.ErrNotFound) {
		if change.ExpectedRevision == 0 {
			return conflict, false, nil
		}
		conflict.Latest, _ = json.Marshal(workspaceSyncOverrideSettingsState{
			TargetID: targetID, RepositoryID: change.RepositoryID,
			Kind: change.Kind, Document: emptyDocument,
		})
		return conflict, true, nil
	}
	if err != nil {
		return conflict, false, err
	}
	conflict.ActualRevision = override.Revision
	if override.Revision == change.ExpectedRevision {
		return conflict, false, nil
	}
	if document, ok := canonicalStoredSyncOverride(change.Kind, override.Document); ok {
		conflict.Latest, _ = json.Marshal(workspaceSyncOverrideSettingsState{
			TargetID: targetID, RepositoryID: override.RepositoryID, Kind: override.Kind,
			Enabled: override.Enabled, Document: document, Revision: override.Revision,
		})
	}

	return conflict, true, nil
}

func canonicalStoredSyncOverride(
	kind orgsync.Kind,
	document []byte,
) (json.RawMessage, bool) {
	if kind != orgsync.KindFiles {
		var empty map[string]json.RawMessage
		if err := json.Unmarshal(documentOrEmpty(document), &empty); err != nil || len(empty) != 0 {
			return nil, false
		}
		return emptyDocument, true
	}
	var override orgsync.FileOverride
	if err := decodeStrictly(document, &override); err != nil {
		return nil, false
	}
	canonical, err := json.Marshal(override)

	return canonical, err == nil
}
