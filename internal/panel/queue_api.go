package panel

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const (
	panelEventQueueChanged = "queue.changed"
	jsonFieldCode          = "code"
	jsonFieldCurrent       = "current"
	jsonFieldMessage       = "message"
	errorCodeStaleRevision = "stale_revision"
)

type queueActionInput struct {
	Type             workqueue.ActionType `json:"type"`
	ExpectedRevision *int64               `json:"expected_revision"`
	Reason           string               `json:"reason"`
	At               *time.Time           `json:"at"`
	OutsideWindow    bool                 `json:"outside_window"`
	Priority         workqueue.Priority   `json:"priority"`
}

type queueDetailResponse struct {
	Item   workqueue.Item    `json:"item"`
	Events []workqueue.Event `json:"events"`
}

type queueActionPreview struct {
	ItemRevision    int64     `json:"item_revision"`
	RequestedAt     time.Time `json:"requested_at"`
	EligibleAt      time.Time `json:"eligible_at"`
	OutsideWindow   bool      `json:"outside_window"`
	ProfileID       *string   `json:"profile_id,omitempty"`
	ProfileName     string    `json:"profile_name,omitempty"`
	ProfileTimezone string    `json:"profile_timezone,omitempty"`
}

func (s *Server) getRootQueue(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	s.writeQueuePage(w, r, nil, true, true)
}

func (s *Server) getTargetQueue(w http.ResponseWriter, r *http.Request) {
	_, target, access, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	canControl := access.Role == storage.InstallationRoleAdmin ||
		access.Role == storage.InstallationRoleOwner
	s.writeQueuePage(w, r, &target.ID, canControl, false)
}

func (s *Server) writeQueuePage(
	w http.ResponseWriter,
	r *http.Request,
	targetID *string,
	canControl bool,
	root bool,
) {
	filter, err := parseQueueFilter(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_queue_query", err.Error())
		return
	}
	if targetID != nil {
		filter.TargetID = targetID
	}
	page, err := s.store.ListWorkQueue(r.Context(), filter)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	for index := range page.Items {
		prepareQueueItem(&page.Items[index], canControl, root)
	}
	writeJSON(w, http.StatusOK, page)
}

func parseQueueFilter(r *http.Request) (workqueue.Filter, error) {
	filter := workqueue.Filter{}
	values := r.URL.Query()
	if raw := strings.TrimSpace(values.Get("installation")); raw != "" {
		filter.TargetID = &raw
	}
	if raw := strings.TrimSpace(values.Get("repository")); raw != "" {
		filter.RepositoryID = &raw
	}
	if raw := strings.TrimSpace(values.Get("profile")); raw != "" {
		filter.ProfileID = &raw
	}
	if err := parseQueueKinds(values, &filter); err != nil {
		return workqueue.Filter{}, err
	}
	if err := parseQueueTimes(values, &filter); err != nil {
		return workqueue.Filter{}, err
	}
	if err := parseQueuePage(values, &filter); err != nil {
		return workqueue.Filter{}, err
	}
	if raw := strings.TrimSpace(values.Get("summary")); raw != "" {
		if raw != "true" {
			return workqueue.Filter{}, errors.New("queue summary must be true")
		}
		filter.Summary = true
	}
	if raw := strings.TrimSpace(values.Get("order")); raw != "" {
		if raw != "dispatch" {
			return workqueue.Filter{}, errors.New("queue order is invalid")
		}
		if !dispatchOrderStates(filter.States) {
			return workqueue.Filter{}, errors.New(
				"dispatch order requires active queue states",
			)
		}
		filter.DispatchOrder = true
	}

	return filter, nil
}

func dispatchOrderStates(states []workqueue.State) bool {
	if len(states) == 0 {
		return false
	}
	for _, state := range states {
		switch state {
		case workqueue.StateScheduled, workqueue.StateBlocked, workqueue.StateReady,
			workqueue.StateRunning, workqueue.StateRetrying:
		default:
			return false
		}
	}

	return true
}

func parseQueueKinds(values url.Values, filter *workqueue.Filter) error {
	for _, raw := range splitQueueValues(values["state"]) {
		state := workqueue.State(raw)
		if !state.Valid() {
			return errors.New("queue state is invalid")
		}
		filter.States = append(filter.States, state)
	}
	for _, raw := range splitQueueValues(values["workload"]) {
		kind := workqueue.Kind(raw)
		if !kind.Valid() {
			return errors.New("queue workload is invalid")
		}
		filter.Kinds = append(filter.Kinds, kind)
	}
	for _, raw := range splitQueueValues(values["priority"]) {
		priority := workqueue.Priority(raw)
		if !priority.Valid() {
			return errors.New("queue priority is invalid")
		}
		filter.Priorities = append(filter.Priorities, priority)
	}

	return nil
}

func parseQueueTimes(values url.Values, filter *workqueue.Filter) error {
	if raw := values.Get("created_after"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return errors.New("queue created_after must be an RFC3339 timestamp")
		}
		value = value.UTC()
		filter.CreatedAfter = &value
	}
	if raw := values.Get("created_before"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return errors.New("queue created_before must be an RFC3339 timestamp")
		}
		value = value.UTC()
		filter.CreatedBefore = &value
	}
	if filter.CreatedAfter != nil && filter.CreatedBefore != nil &&
		!filter.CreatedAfter.Before(*filter.CreatedBefore) {
		return errors.New("queue created_after must be before created_before")
	}

	return nil
}

func parseQueuePage(values url.Values, filter *workqueue.Filter) error {
	if raw := values.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 200 {
			return errors.New("queue limit must be between 1 and 200")
		}
		filter.Limit = value
	}
	if raw := values.Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return errors.New("queue offset must be non-negative")
		}
		filter.Offset = value
	}

	return nil
}

func splitQueueValues(values []string) []string {
	var result []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}

	return result
}

func (s *Server) getRootQueueItem(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	s.writeQueueDetail(w, r, nil, true, true)
}

func (s *Server) getTargetQueueItem(w http.ResponseWriter, r *http.Request) {
	_, target, access, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	canControl := access.Role == storage.InstallationRoleAdmin ||
		access.Role == storage.InstallationRoleOwner
	s.writeQueueDetail(w, r, &target.ID, canControl, false)
}

func (s *Server) writeQueueDetail(
	w http.ResponseWriter,
	r *http.Request,
	targetID *string,
	canControl bool,
	root bool,
) {
	item, err := s.store.GetQueueItem(r.Context(), r.PathValue("queue"))
	if err != nil || targetID != nil && (item.TargetID == nil || *item.TargetID != *targetID) {
		if err == nil {
			err = storage.ErrNotFound
		}
		s.writeStorageError(w, err)
		return
	}
	events, err := s.store.ListQueueEvents(r.Context(), item.ID, 200)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	prepareQueueItem(&item, canControl, root)
	if !root {
		redactInstallationQueueEvents(events)
	}
	writeJSON(w, http.StatusOK, queueDetailResponse{Item: item, Events: events})
}

func prepareQueueItem(item *workqueue.Item, canControl, root bool) {
	if !root {
		redactInstallationQueueItem(item)
	}
	if !canControl || item.State.Terminal() {
		item.Actions = nil
		return
	}
	item.Actions = []workqueue.ActionType{workqueue.ActionSetPriority}
	if item.State == workqueue.StateRunning || item.State == workqueue.StateAwaitingApproval {
		return
	}
	item.Actions = append(item.Actions, workqueue.ActionRunNow)
	if item.Kind.Windowed() {
		item.Actions = append(item.Actions, workqueue.ActionNextWindow, workqueue.ActionScheduleAt)
	}
	if item.SourceKind == "" || item.SourceKind == "recurring" {
		item.Actions = append(item.Actions, workqueue.ActionCancel)
	}
}

func redactInstallationQueueItem(item *workqueue.Item) {
	if item.Kind == workqueue.KindWebhookDelivery {
		item.Details = nil
	}
	switch item.State {
	case workqueue.StateFailed:
		item.BlockedReason = "Work failed. Root can inspect the retained failure detail."
	case workqueue.StateRetrying:
		item.BlockedReason = "Work will retry. Root can inspect the retained failure detail."
	}
}

func redactInstallationQueueEvents(events []workqueue.Event) {
	for index := range events {
		events[index].Details = nil
		switch events[index].State {
		case workqueue.StateFailed:
			events[index].Summary = "Work failed. Root can inspect the retained failure detail."
		case workqueue.StateRetrying:
			events[index].Summary = "Work will retry. Root can inspect the retained failure detail."
		}
	}
}

func (s *Server) postRootQueueAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	s.applyQueueAction(w, r, account, nil, true)
}

func (s *Server) previewRootQueueAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	s.previewQueueAction(w, r, nil)
}

func (s *Server) previewTargetQueueAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	_, target, access, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	if access.Role != storage.InstallationRoleAdmin && access.Role != storage.InstallationRoleOwner {
		s.writeError(w, http.StatusForbidden, "forbidden", "Admin or Owner access is required")
		return
	}
	s.previewQueueAction(w, r, &target.ID)
}

func (s *Server) previewQueueAction(w http.ResponseWriter, r *http.Request, targetID *string) {
	var input queueActionInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Type != workqueue.ActionScheduleAt || input.At == nil || input.ExpectedRevision == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_queue_preview",
			"schedule_at, exact time, and expected_revision are required")
		return
	}
	item, err := s.store.GetQueueItem(r.Context(), r.PathValue("queue"))
	if err != nil || targetID != nil && (item.TargetID == nil || *item.TargetID != *targetID) {
		if err == nil {
			err = storage.ErrNotFound
		}
		s.writeStorageError(w, err)
		return
	}
	if item.Revision != *input.ExpectedRevision {
		prepareQueueItem(&item, true, targetID == nil)
		writeJSON(w, http.StatusConflict, map[string]any{
			jsonFieldCode: errorCodeStaleRevision, jsonFieldMessage: "queue item changed; review the latest state",
			jsonFieldCurrent: item,
		})
		return
	}
	preview := queueActionPreview{
		ItemRevision: item.Revision, RequestedAt: input.At.UTC(),
		EligibleAt: input.At.UTC(), OutsideWindow: input.OutsideWindow,
		ProfileID: item.ProfileID,
	}
	if !input.OutsideWindow {
		if item.ProfileID == nil {
			s.writeError(w, http.StatusConflict, "missing_profile", "queue item has no schedule profile")
			return
		}
		profile, profileErr := s.store.GetScheduleProfile(r.Context(), *item.ProfileID)
		if profileErr != nil {
			s.writeStorageError(w, profileErr)
			return
		}
		preview.EligibleAt, err = workqueue.NextEligible(profile, input.At.UTC())
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error())
			return
		}
		preview.ProfileName, preview.ProfileTimezone = profile.Name, profile.Timezone
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) postTargetQueueAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, access, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	if access.Role != storage.InstallationRoleAdmin && access.Role != storage.InstallationRoleOwner {
		s.writeError(w, http.StatusForbidden, "forbidden", "Admin or Owner access is required")
		return
	}
	s.applyQueueAction(w, r, account, &target.ID, false)
}

func (s *Server) applyQueueAction(
	w http.ResponseWriter,
	r *http.Request,
	account storage.Account,
	targetID *string,
	root bool,
) {
	var input queueActionInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.ExpectedRevision == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "expected_revision is required")
		return
	}
	if message := validateQueueActionInput(input); message != "" {
		s.writeError(w, http.StatusBadRequest, "invalid_queue_action", message)
		return
	}
	id := r.PathValue("queue")
	current, err := s.store.GetQueueItem(r.Context(), id)
	if err != nil || targetID != nil && (current.TargetID == nil || *current.TargetID != *targetID) {
		if err == nil {
			err = storage.ErrNotFound
		}
		s.writeStorageError(w, err)
		return
	}
	if !queueActionAllowed(current, input.Type) {
		s.writeError(w, http.StatusConflict, "unsupported_action", "the action is not available in the current state")
		return
	}
	action := workqueue.ItemAction{
		Type: input.Type, ExpectedRevision: *input.ExpectedRevision,
		ActorID: account.ID, Reason: input.Reason, OutsideWindow: input.OutsideWindow,
		Priority: input.Priority, ChangedAt: s.now().UTC(),
	}
	if input.At != nil {
		action.At = input.At.UTC()
	}
	item, err := s.store.ApplyQueueAction(r.Context(), id, action)
	if errors.Is(err, storage.ErrConflict) {
		latest, latestErr := s.store.GetQueueItem(r.Context(), id)
		if latestErr == nil {
			prepareQueueItem(&latest, true, root)
			writeJSON(w, http.StatusConflict, map[string]any{
				jsonFieldCode: errorCodeStaleRevision, jsonFieldMessage: "queue item changed; review the latest state",
				jsonFieldCurrent: latest,
			})
			return
		}
	}
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	prepareQueueItem(&item, true, root)
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: queueTarget(item)})
	if s.queue != nil {
		s.queue.WakeQueue(item.Lane)
	} else if item.Lane != workqueue.LaneWebhook {
		s.pendingCI.Wake()
	}
	writeJSON(w, http.StatusOK, item)
}

func validateQueueActionInput(input queueActionInput) string {
	switch input.Type {
	case workqueue.ActionRunNow:
		if strings.TrimSpace(input.Reason) == "" {
			return "run now requires a reason"
		}
	case workqueue.ActionScheduleAt:
		if input.At == nil {
			return "schedule_at requires an exact time"
		}
		if input.OutsideWindow && strings.TrimSpace(input.Reason) == "" {
			return "outside-window scheduling requires a reason"
		}
	case workqueue.ActionSetPriority:
		if !input.Priority.Valid() {
			return "priority is invalid"
		}
	case workqueue.ActionNextWindow, workqueue.ActionCancel:
	default:
		return "action type is invalid"
	}

	return ""
}

func queueActionAllowed(item workqueue.Item, action workqueue.ActionType) bool {
	prepareQueueItem(&item, true, true)
	for _, allowed := range item.Actions {
		if allowed == action {
			return true
		}
	}

	return false
}

func queueTarget(item workqueue.Item) string {
	if item.TargetID == nil {
		return ""
	}

	return *item.TargetID
}
