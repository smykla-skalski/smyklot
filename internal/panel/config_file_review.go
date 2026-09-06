package panel

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/smykla-skalski/smyklot/internal/configsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// ConfigFileController shares the service's configuration exclusion and GitHub
// credentials. A resolution re-reads both sides before saving its reviewed choice.
type ConfigFileController interface {
	PreviewConfigurationFile(context.Context, string, string) (configsync.ConnectionPreview, error)
	ResolveConfigurationFile(context.Context, ConfigFileResolutionRequest) error
}

type ConfigFileResolutionRequest struct {
	TargetID         string
	RepositoryID     string
	ReviewToken      string
	Side             string
	ActorAccountID   string
	ElevationID      *string
	SessionTokenHash string
}

func (s *Server) getConfigFilePreview(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if ok {
		s.writeConfigFilePreview(w, r, target.ID)
	}
}

func (s *Server) getRootConfigFilePreview(w http.ResponseWriter, r *http.Request) {
	root, ok := s.requireRootTarget(w, r, false)
	if ok {
		s.writeConfigFilePreview(w, r, root.Target.ID)
	}
}

func (s *Server) configFileReviewAvailable(w http.ResponseWriter, r *http.Request, targetID string) bool {
	if repositoryID := r.PathValue("repository"); repositoryID != "" {
		if _, err := s.store.GetRepository(r.Context(), targetID, repositoryID); err != nil {
			s.writeStorageError(w, err)
			return false
		}
	}
	if s.configFiles == nil {
		s.writeError(w, http.StatusServiceUnavailable, "config_file_unavailable", "Configuration file review is unavailable")
		return false
	}
	return true
}

func (s *Server) writeConfigFilePreview(w http.ResponseWriter, r *http.Request, targetID string) {
	if !s.configFileReviewAvailable(w, r, targetID) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	preview, err := s.configFiles.PreviewConfigurationFile(ctx, targetID, r.PathValue("repository"))
	if err != nil {
		s.writeConfigFileReviewError(w, err, s.writeStorageError)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) postConfigFileResolution(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if ok {
		s.saveConfigFileResolution(w, r, ConfigFileResolutionRequest{TargetID: target.ID, ActorAccountID: account.ID}, s.writeStorageError)
	}
}

func (s *Server) postRootConfigFileResolution(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	root, ok := s.requireRootTarget(w, r, true)
	if ok {
		s.saveConfigFileResolution(w, r, ConfigFileResolutionRequest{
			TargetID: root.Target.ID, ActorAccountID: root.Account.ID,
			ElevationID: elevationID(root.Elevation), SessionTokenHash: root.SessionHash,
		}, s.writeRootWriteError)
	}
}

func (s *Server) saveConfigFileResolution(w http.ResponseWriter, r *http.Request, request ConfigFileResolutionRequest, writeFailure func(http.ResponseWriter, error)) {
	if !s.configFileReviewAvailable(w, r, request.TargetID) {
		return
	}
	var input struct {
		ReviewToken string `json:"review_token"`
		Side        string `json:"side"`
	}
	if !decodeJSONWithin(w, r, &input, 4096) {
		return
	}
	token, err := hex.DecodeString(input.ReviewToken)
	if err != nil || len(token) != 32 || (input.Side != configsync.ResolutionPanel && input.Side != configsync.ResolutionFile) {
		s.writeError(w, http.StatusBadRequest, "invalid_config_file_choice", "Review the current file and choose which conflicting values to keep")
		return
	}
	request.RepositoryID, request.ReviewToken, request.Side = r.PathValue("repository"), input.ReviewToken, input.Side
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.configFiles.ResolveConfigurationFile(ctx, request); err != nil {
		s.writeConfigFileReviewError(w, err, writeFailure)
		return
	}
	if s.queue != nil {
		s.queue.WakeQueue(workqueue.LaneMaintenance)
	}
	s.Announce(request.TargetID, request.RepositoryID)
	writeJSON(w, http.StatusAccepted, struct {
		Status string `json:"status"`
	}{Status: "pending"})
}

func (s *Server) writeConfigFileReviewError(w http.ResponseWriter, err error, writeFailure func(http.ResponseWriter, error)) {
	var blocked *configsync.BlockedError
	switch {
	case errors.Is(err, storage.ErrConflict):
		s.writeError(w, http.StatusConflict, "config_file_changed", "Settings or the file changed. Review the current values before choosing again")
	case errors.As(err, &blocked):
		s.writeError(w, http.StatusUnprocessableEntity, blocked.Code, blocked.Message)
	default:
		writeFailure(w, err)
	}
}
