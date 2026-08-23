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
const maxInstallationSettingsBatchBody = 8 << 20

type installationSettingsBatchActor struct {
	accountID        string
	elevationID      *string
	sessionTokenHash string
	writeError       func(http.ResponseWriter, error)
}

func (s *Server) putInstallationSettingsBatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}
	s.putAuthorizedInstallationSettingsBatch(w, r, target, installationSettingsBatchActor{
		accountID: account.ID, writeError: s.writeStorageError,
	})
}

func (s *Server) putRootInstallationSettingsBatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	root, ok := s.requireRootTarget(w, r, true)
	if !ok {
		return
	}
	s.putAuthorizedInstallationSettingsBatch(w, r, root.Target, installationSettingsBatchActor{
		accountID: root.Account.ID, elevationID: elevationID(root.Elevation),
		sessionTokenHash: root.SessionHash, writeError: s.writeRootWriteError,
	})
}

func (s *Server) putAuthorizedInstallationSettingsBatch(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
	actor installationSettingsBatchActor,
) {
	var input installationSettingsBatchRequest
	if !decodeJSONWithin(w, r, &input, maxInstallationSettingsBatchBody) {
		return
	}
	request, err := s.prepareInstallationSettingsBatch(r, target, actor, input)
	if err != nil {
		s.writeInstallationSettingsBatchPreparationError(w, err, actor.writeError)
		return
	}
	result, err := s.saveInstallationSettingsBatch(r.Context(), request)
	if err != nil {
		if errors.Is(err, orgsync.ErrInvalidConfig) {
			s.writeError(w, http.StatusBadRequest, "invalid_sync_config", err.Error())
			return
		}
		if errors.Is(err, storage.ErrConflict) &&
			s.writeInstallationSettingsBatchConflict(w, r.Context(), request) {
			return
		}
		actor.writeError(w, err)
		return
	}
	answer := installationSettingsBatchAnswer(request, result)
	s.signalInstallationSettingsBatch(target.ID, result)
	writeJSON(w, http.StatusOK, answer)
}

// saveInstallationSettingsBatch holds the Pending CI exclusion while calling
// the storage transaction exactly once.
func (s *Server) saveInstallationSettingsBatch(
	ctx context.Context,
	request storage.SaveInstallationSettingsRequest,
) (storage.SaveInstallationSettingsResult, error) {
	operation := func() (storage.SaveInstallationSettingsResult, error) {
		return s.store.SaveInstallationSettings(ctx, request)
	}
	if request.Target != nil {
		return s.saveInstallationTargetSettingsBatch(ctx, request.TargetID, operation)
	}
	if len(request.Repositories) == 0 {
		return operation()
	}
	repositoryIDs := make([]string, 0, len(request.Repositories))
	for _, repository := range request.Repositories {
		repositoryIDs = append(repositoryIDs, repository.RepositoryID)
	}
	sort.Strings(repositoryIDs)

	return saveInstallationSettingsExclusive(s, ctx, repositoryIDs, operation)
}

func (s *Server) saveInstallationTargetSettingsBatch(
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
		result, saveErr = saveInstallationSettingsExclusive(s, ctx, repositoryIDs, operation)

		return saveErr
	})

	return result, err
}

func saveInstallationSettingsExclusive(
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

func (s *Server) signalInstallationSettingsBatch(
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
