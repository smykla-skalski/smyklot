package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

type elevationRequest struct {
	Acknowledged *bool   `json:"acknowledged"`
	Reason       *string `json:"reason"`
}

func (s *Server) getRootOverview(w http.ResponseWriter, r *http.Request) {
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	now := s.now().UTC()

	// Described rather than pinged: the answer to "is it there" is one field of
	// the answer to "what is it", and asking twice would let the page report a
	// database it had checked and a database it had read.
	database := s.store.Status(r.Context())
	if !database.Reachable {
		s.writeInternal(w, errors.New(database.Error))
		return
	}
	overview, err := s.store.GetRootOverview(r.Context(), account.ID, now)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	active := pendingci.ScheduleActive
	activeQueue, err := s.store.ListQueue(
		r.Context(), pendingci.QueueFilter{Schedule: &active, Limit: 50},
	)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	deferred := pendingci.ScheduleDeferred
	deferredQueue, err := s.store.ListQueue(
		r.Context(), pendingci.QueueFilter{Schedule: &deferred, Limit: 50},
	)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	recent, err := s.store.ListHistory(
		r.Context(), pendingci.HistoryFilter{Limit: 10},
	)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		rootOverviewDTO(
			overview,
			database,
			activeQueue,
			deferredQueue,
			recent,
			s.cfg,
			s.startedAt,
			now,
		),
	)
}

func (s *Server) getRootHistory(w http.ResponseWriter, r *http.Request) {
	history := r.PathValue("history")
	orders := auditHistoryOrders
	if history == panelHistoryFailuresPath {
		orders = failureHistoryOrders
	} else if history != panelHistoryAuditPath {
		s.writeError(w, http.StatusNotFound, "not_found", "Root history view was not found")
		return
	}
	page, ok := s.rootHistoryPage(w, r, orders...)
	if !ok {
		return
	}
	if history == panelHistoryFailuresPath {
		s.getRootFailurePage(w, r, page)
		return
	}
	s.getRootAuditPage(w, r, page, nil)
}

func (s *Server) getRootAuditPage(
	w http.ResponseWriter,
	r *http.Request,
	page storage.HistoryPageRequest,
	targetID *string,
) {
	categories, err := parseRootAuditCategories(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", err.Error())
		return
	}
	result, err := s.store.ListRootAudit(r.Context(), storage.RootAuditPageRequest{
		HistoryPageRequest: page, Categories: categories, TargetID: targetID,
	})
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rootAuditPageDTO(result))
}

func (s *Server) getRootFailurePage(
	w http.ResponseWriter,
	r *http.Request,
	page storage.HistoryPageRequest,
) {
	retryable, err := parseRootFailureKind(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", err.Error())
		return
	}
	result, err := s.store.ListRootFailures(r.Context(), storage.FailurePageRequest{
		HistoryPageRequest: page, Retryable: retryable,
	})
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rootFailurePageDTO(result))
}

func (s *Server) rootHistoryPage(
	w http.ResponseWriter,
	r *http.Request,
	orders ...storage.HistoryOrder,
) (storage.HistoryPageRequest, bool) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return storage.HistoryPageRequest{}, false
	}
	page, err := parseHistoryPage(r.URL.Query(), orders...)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", err.Error())
		return storage.HistoryPageRequest{}, false
	}

	return page, true
}

func parseRootAuditCategories(r *http.Request) ([]storage.AuditCategory, error) {
	raw := r.URL.Query()["category"]
	if len(raw) == 0 || (len(raw) == 1 && (raw[0] == "" || raw[0] == allFilter)) {
		return nil, nil
	}
	categories := make([]storage.AuditCategory, 0, len(raw))
	for _, value := range raw {
		category := storage.AuditCategory(value)
		switch category {
		case storage.AuditCategoryConfiguration, storage.AuditCategoryAccess,
			storage.AuditCategoryOwnership, storage.AuditCategoryElevation,
			storage.AuditCategoryNotification, storage.AuditCategoryRuntime:
			categories = append(categories, category)
		default:
			return nil, fmt.Errorf("invalid audit category")
		}
	}

	return categories, nil
}

func parseRootFailureKind(r *http.Request) (*bool, error) {
	switch r.URL.Query().Get("kind") {
	case "", allFilter:
		return nil, nil
	case "retryable":
		value := true
		return &value, nil
	case "permanent":
		value := false
		return &value, nil
	default:
		return nil, fmt.Errorf("invalid failure kind")
	}
}

func (s *Server) getRootWorkspaces(w http.ResponseWriter, r *http.Request) {
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	targets, err := s.store.ListRootTargets(r.Context())
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	now := s.now().UTC()
	items := make([]rootWorkspaceResponse, 0, len(targets))
	for _, target := range targets {
		owned := false
		if target.Available {
			access, accessErr := s.store.ResolveTargetAccess(r.Context(), account.ID, target.ID, now)
			if accessErr != nil {
				s.writeInternal(w, accessErr)
				return
			}
			owned = access.Role == storage.InstallationRoleOwner
		}
		items = append(items, rootWorkspaceDTO(target, now, owned))
	}
	writeJSON(w, http.StatusOK, map[string]any{panelWorkspacesResource: items})
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
		s.writeError(w, http.StatusBadRequest, "acknowledgment_required", "confirm the operator visit warning")
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
	s.events.announce(panelEvent{Type: panelEventResync})
	s.announceElevationExpiry(elevation)
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
	s.events.announce(panelEvent{Type: panelEventResync})
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
	s.events.announce(panelEvent{Type: panelEventResync})
	writeJSON(w, http.StatusOK, securityNotificationDTO(notification))
}

// putSecurityNotificationsAllRead empties one reader's inbox. It answers the count
// rather than the rows: the page that calls this already holds the list and refetches
// it, and sending every notification back to say the same word about each is a page of
// JSON to report a single fact.
func (s *Server) putSecurityNotificationsAllRead(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, ok := s.requireViewer(w, r)
	if !ok {
		return
	}
	cleared, err := s.store.MarkAllSecurityNotificationsRead(r.Context(), account.ID, s.now().UTC())
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventResync})
	writeJSON(w, http.StatusOK, map[string]int{"read": cleared})
}

func (s *Server) announceElevationExpiry(elevation storage.Elevation) {
	delay := elevation.ExpiresAt.Sub(s.now().UTC())
	if delay <= 0 {
		_, _ = s.store.EndElevation(
			context.Background(), elevation.ID, elevation.SessionTokenHash,
			storage.ElevationExpired, s.now().UTC(),
		)
		s.events.announce(panelEvent{Type: panelEventResync})
		return
	}
	time.AfterFunc(delay, func() {
		_, _ = s.store.EndElevation(
			context.Background(), elevation.ID, elevation.SessionTokenHash,
			storage.ElevationExpired, s.now().UTC(),
		)
		s.events.announce(panelEvent{Type: panelEventResync})
	})
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
		s.writeError(w, http.StatusNotFound, "not_found", "no operator visit to this workspace was found")
	case errors.Is(err, storage.ErrConflict):
		s.writeError(w, http.StatusConflict, "conflict", "this workspace cannot be visited right now")
	case errors.Is(err, storage.ErrExpired), errors.Is(err, storage.ErrRevoked):
		s.writeError(w, http.StatusGone, "elevation_expired", "the operator visit has ended")
	default:
		s.writeInternal(w, err)
	}
}
