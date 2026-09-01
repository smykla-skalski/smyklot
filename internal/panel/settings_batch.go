package panel

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// A batch may contain the one files document, plus the smaller documents and
// repository settings around it. It is deliberately finite even though every
// resource inside it is bounded again during preflight.
const maxWorkspaceSettingsBatchBody = 8 << 20

type workspaceSettingsBatchActor struct {
	accountID        string
	elevationID      *string
	sessionTokenHash string
	writeError       func(http.ResponseWriter, error)
}

func (s *Server) putWorkspaceSettingsBatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}
	s.putAuthorizedWorkspaceSettingsBatch(w, r, target, workspaceSettingsBatchActor{
		accountID: account.ID, writeError: s.writeStorageError,
	})
}

func (s *Server) putRootWorkspaceSettingsBatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	root, ok := s.requireRootTarget(w, r, true)
	if !ok {
		return
	}
	s.putAuthorizedWorkspaceSettingsBatch(w, r, root.Target, workspaceSettingsBatchActor{
		accountID: root.Account.ID, elevationID: elevationID(root.Elevation),
		sessionTokenHash: root.SessionHash, writeError: s.writeRootWriteError,
	})
}

func (s *Server) putAuthorizedWorkspaceSettingsBatch(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
	actor workspaceSettingsBatchActor,
) {
	var input workspaceSettingsBatchRequest
	if !decodeJSONWithin(w, r, &input, maxWorkspaceSettingsBatchBody) {
		return
	}
	request, err := s.prepareWorkspaceSettingsBatch(r, target, actor, input)
	if err != nil {
		s.writeWorkspaceSettingsBatchPreparationError(w, err, actor.writeError)
		return
	}
	result, err := s.saveWorkspaceSettingsBatch(r.Context(), request)
	if err != nil {
		if errors.Is(err, orgsync.ErrInvalidConfig) {
			s.writeError(w, http.StatusBadRequest, "invalid_sync_config", err.Error())
			return
		}
		if errors.Is(err, storage.ErrConflict) &&
			s.writeWorkspaceSettingsBatchConflict(w, r.Context(), request) {
			return
		}
		actor.writeError(w, err)
		return
	}
	answer := workspaceSettingsBatchAnswer(request, result)
	s.signalWorkspaceSettingsBatch(target.ID, result)
	writeJSON(w, http.StatusOK, answer)
}

// saveWorkspaceSettingsBatch holds the Pending CI exclusion while calling
// the storage transaction exactly once.
func (s *Server) saveWorkspaceSettingsBatch(
	ctx context.Context,
	request storage.SaveInstallationSettingsRequest,
) (storage.SaveInstallationSettingsResult, error) {
	operation := func() (storage.SaveInstallationSettingsResult, error) {
		return s.store.SaveInstallationSettings(ctx, request)
	}
	if request.Target != nil {
		return s.saveWorkspaceTargetSettingsBatch(ctx, request.TargetID, operation)
	}
	if len(request.Repositories) == 0 {
		return operation()
	}
	repositoryIDs := make([]string, 0, len(request.Repositories))
	for _, repository := range request.Repositories {
		repositoryIDs = append(repositoryIDs, repository.RepositoryID)
	}
	sort.Strings(repositoryIDs)

	return saveWorkspaceSettingsExclusive(s, ctx, repositoryIDs, operation)
}

func (s *Server) saveWorkspaceTargetSettingsBatch(
	ctx context.Context,
	targetID string,
	operation func() (storage.SaveInstallationSettingsResult, error),
) (storage.SaveInstallationSettingsResult, error) {
	var result storage.SaveInstallationSettingsResult
	err := s.pendingCI.ExclusiveCatalog(ctx, func() error {
		repositories, listErr := s.store.ListRepositories(ctx, targetID)
		if listErr != nil {
			return listErr
		}
		repositoryIDs := make([]string, 0, len(repositories))
		for _, repository := range repositories {
			repositoryIDs = append(repositoryIDs, repository.ID)
		}
		sort.Strings(repositoryIDs)
		var saveErr error
		result, saveErr = saveWorkspaceSettingsExclusive(s, ctx, repositoryIDs, operation)

		return saveErr
	})

	return result, err
}

func saveWorkspaceSettingsExclusive(
	s *Server,
	ctx context.Context,
	repositoryIDs []string,
	operation func() (storage.SaveInstallationSettingsResult, error),
) (storage.SaveInstallationSettingsResult, error) {
	var result storage.SaveInstallationSettingsResult
	err := s.pendingCI.Exclusive(ctx, repositoryIDs, func() error {
		var saveErr error
		result, saveErr = operation()

		return saveErr
	})

	return result, err
}

func (s *Server) signalWorkspaceSettingsBatch(
	targetID string,
	result storage.SaveInstallationSettingsResult,
) {
	if result.CheckpointID == nil {
		return
	}
	if result.CatalogSettingsChanged {
		s.pendingCI.Wake()
		s.wakePendingCIGates()
	}
	s.Announce(targetID, "")
}
