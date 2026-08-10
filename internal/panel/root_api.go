package panel

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

type elevationRequest struct {
	Acknowledged *bool   `json:"acknowledged"`
	Reason       *string `json:"reason"`
}

func (s *Server) getRootInstallations(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	targets, err := s.store.ListRootTargets(r.Context())
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	now := s.now().UTC()
	items := make([]rootInstallationResponse, 0, len(targets))
	for _, target := range targets {
		items = append(items, rootInstallationDTO(target, now))
	}
	writeJSON(w, http.StatusOK, map[string]any{panelInstallationsResource: items})
}

func (s *Server) postRootElevation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, sessionHash, ok := s.requireRootSession(w, r)
	if !ok {
		return
	}
	var input elevationRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Acknowledged == nil || !*input.Acknowledged {
		s.writeError(w, http.StatusBadRequest, "acknowledgment_required", "confirm the elevated access warning")
		return
	}
	reason, ok := validAccessReason(w, input.Reason)
	if !ok {
		return
	}
	id, err := randomToken(s.random)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	elevation, err := s.store.BeginElevation(r.Context(), storage.ElevationGrant{
		ID: id, SessionTokenHash: sessionHash, RootAccountID: account.ID,
		TargetID: r.PathValue("target"), Reason: reason, StartedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeElevationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, elevationDTO(elevation))
}

func (s *Server) getRootElevation(w http.ResponseWriter, r *http.Request) {
	_, _, sessionHash, ok := s.requireRootSession(w, r)
	if !ok {
		return
	}
	elevation, err := s.store.GetElevation(
		r.Context(), sessionHash, r.PathValue("target"), s.now().UTC(),
	)
	if err != nil {
		s.writeElevationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, elevationDTO(elevation))
}

func (s *Server) deleteRootElevation(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	_, _, sessionHash, ok := s.requireRootSession(w, r)
	if !ok {
		return
	}
	elevation, err := s.store.EndElevation(
		r.Context(), r.PathValue("elevation"), sessionHash,
		storage.ElevationEnded, s.now().UTC(),
	)
	if err != nil {
		s.writeElevationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, elevationDTO(elevation))
}

func (s *Server) getSecurityNotifications(w http.ResponseWriter, r *http.Request) {
	account, ok := s.requireViewer(w, r)
	if !ok {
		return
	}
	page, err := parseNotificationPage(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_notification_query", err.Error())
		return
	}
	notifications, err := s.store.ListSecurityNotifications(r.Context(), account.ID, page)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notificationPageDTO(notifications))
}

func (s *Server) putSecurityNotificationRead(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, ok := s.requireViewer(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("notification"), 10, 64)
	if err != nil || id <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "notification ID is invalid")
		return
	}
	notification, err := s.store.MarkSecurityNotificationRead(
		r.Context(), account.ID, id, s.now().UTC(),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, securityNotificationDTO(notification))
}

func parseNotificationPage(r *http.Request) (storage.NotificationPageRequest, error) {
	page := storage.NotificationPageRequest{Limit: DefaultPageSize}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset <= 0 {
			return storage.NotificationPageRequest{}, fmt.Errorf("invalid notification cursor")
		}
		page.Offset = offset
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > MaxPageSize {
			return storage.NotificationPageRequest{}, fmt.Errorf("invalid notification page size")
		}
		page.Limit = limit
	}

	return page, nil
}

func (s *Server) writeElevationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "not_found", "elevated installation access was not found")
	case errors.Is(err, storage.ErrConflict):
		s.writeError(w, http.StatusConflict, "conflict", "elevated access is unavailable for this installation")
	case errors.Is(err, storage.ErrExpired), errors.Is(err, storage.ErrRevoked):
		s.writeError(w, http.StatusGone, "elevation_expired", "elevated installation access has ended")
	default:
		s.writeInternal(w, err)
	}
}
