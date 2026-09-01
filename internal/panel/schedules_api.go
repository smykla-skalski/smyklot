package panel

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

type scheduleProfileInput struct {
	Name             string                `json:"name"`
	Timezone         string                `json:"timezone"`
	Windows          []workqueue.Window    `json:"windows"`
	Exceptions       []workqueue.Exception `json:"exceptions"`
	ExpectedRevision int64                 `json:"expected_revision"`
}

type queuePolicyInput struct {
	Enabled          bool               `json:"enabled"`
	CadenceSeconds   int64              `json:"cadence_seconds"`
	ProfileID        string             `json:"profile_id"`
	DefaultPriority  workqueue.Priority `json:"default_priority"`
	RetryDelay       int64              `json:"retry_delay_seconds"`
	Retention        *int64             `json:"retention_seconds"`
	ApprovalLifetime *int64             `json:"approval_lifetime_seconds"`
	Configuration    json.RawMessage    `json:"configuration"`
	ExpectedRevision int64              `json:"expected_revision"`
}

type scheduleRequestInput struct {
	Kind            workqueue.Kind     `json:"kind"`
	BaseRevision    int64              `json:"base_revision"`
	ProfileID       *string            `json:"profile_id"`
	CustomProfile   *workqueue.Profile `json:"custom_profile"`
	CadenceSeconds  int64              `json:"cadence_seconds"`
	DefaultPriority workqueue.Priority `json:"default_priority"`
	Configuration   json.RawMessage    `json:"configuration"`
	Reason          string             `json:"reason"`
}

type scheduleDecisionInput struct {
	Approve          bool    `json:"approve"`
	PromoteProfile   bool    `json:"promote_profile"`
	ProfileID        *string `json:"profile_id"`
	Reason           string  `json:"reason"`
	ExpectedRevision *int64  `json:"expected_revision"`
}

type schedulePolicySet struct {
	Current            []workqueue.Policy `json:"current"`
	DeploymentDefaults []workqueue.Policy `json:"deployment_defaults"`
	Overrides          []workqueue.Policy `json:"overrides"`
	Effective          []workqueue.Policy `json:"effective"`
}

func newSchedulePolicySet(defaults []workqueue.Policy) schedulePolicySet {
	return schedulePolicySet{
		Current:            []workqueue.Policy{},
		DeploymentDefaults: defaults,
		Overrides:          []workqueue.Policy{},
		Effective:          []workqueue.Policy{},
	}
}

func (s *Server) getRootScheduleProfiles(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	profiles, err := s.store.ListScheduleProfiles(r.Context(), r.URL.Query().Get("archived") == "true")
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

func (s *Server) postRootScheduleProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	var input scheduleProfileInput
	if !decodeJSON(w, r, &input) {
		return
	}
	id, err := randomToken(s.random)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	s.saveRootScheduleProfile(w, r, account, "profile:"+id, input, http.StatusCreated)
}

func (s *Server) putRootScheduleProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	var input scheduleProfileInput
	if !decodeJSON(w, r, &input) {
		return
	}
	s.saveRootScheduleProfile(w, r, account, r.PathValue("profile"), input, http.StatusOK)
}

func (s *Server) saveRootScheduleProfile(
	w http.ResponseWriter,
	r *http.Request,
	account storage.Account,
	id string,
	input scheduleProfileInput,
	status int,
) {
	now := s.now().UTC()
	profile, err := s.store.SaveScheduleProfile(r.Context(), workqueue.ProfileChange{
		ID: id, Name: input.Name, Timezone: input.Timezone,
		Windows: input.Windows, Exceptions: input.Exceptions,
		ExpectedRevision: input.ExpectedRevision, ActorID: account.ID, ChangedAt: now,
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventQueueChanged})
	s.wakeScheduledWork(workqueue.LaneMaintenance)
	s.wakeScheduledWork(workqueue.LanePendingCI)
	writeJSON(w, status, profile)
}

func (s *Server) deleteRootScheduleProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	profile, err := s.store.ArchiveScheduleProfile(
		r.Context(), r.PathValue("profile"), revision, account.ID, s.now().UTC(),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) getRootJobPolicies(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	policies, err := s.store.ListAllQueuePolicies(r.Context())
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	set := newSchedulePolicySet(s.deploymentQueuePolicies())
	for _, policy := range policies {
		if policy.TargetID == nil {
			set.Current = append(set.Current, policy)
			set.Effective = append(set.Effective, policy)
		} else {
			set.Overrides = append(set.Overrides, policy)
		}
	}
	statuses, err := s.store.ListQueuePolicyStatuses(r.Context(), nil)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"policies": set.Current, "policy_set": set, "statuses": statuses,
	})
}

func (s *Server) putRootJobPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	kind := workqueue.Kind(r.PathValue("kind"))
	var input queuePolicyInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validQueuePolicyInput(kind, input) {
		s.writeError(w, http.StatusBadRequest, "invalid_policy", "schedule policy is invalid")
		return
	}
	policy, err := s.store.SaveQueuePolicy(r.Context(), policyChange(kind, nil, input, account.ID, s.now().UTC()))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventQueueChanged})
	s.wakeScheduledWork(kind.Lane())
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) putRootWorkspaceJobPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	target, err := s.store.GetTarget(r.Context(), r.PathValue("target"))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	kind := workqueue.Kind(r.PathValue("kind"))
	var input queuePolicyInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if !kind.WorkspaceConfigurable() || !validQueuePolicyInput(kind, input) {
		s.writeError(w, http.StatusBadRequest, "invalid_policy", "the workspace schedule policy is invalid")
		return
	}
	policy, err := s.store.SaveQueuePolicy(r.Context(), policyChange(
		kind, &target.ID, input, account.ID, s.now().UTC(),
	))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: target.ID})
	s.wakeScheduledWork(kind.Lane())
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) deleteRootWorkspaceJobPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	target, err := s.store.GetTarget(r.Context(), r.PathValue("target"))
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	kind := workqueue.Kind(r.PathValue("kind"))
	policy, err := s.store.DeleteQueuePolicyOverride(
		r.Context(), kind, target.ID, revision, account.ID, s.now().UTC(),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: target.ID})
	s.wakeScheduledWork(kind.Lane())
	writeJSON(w, http.StatusOK, policy)
}

func validQueuePolicyInput(kind workqueue.Kind, input queuePolicyInput) bool {
	return kind.Valid() && kind != workqueue.KindScheduleChange && input.CadenceSeconds >= 0 &&
		(!input.Enabled || !kind.Recurring() || input.CadenceSeconds > 0) &&
		input.RetryDelay >= 0 && input.DefaultPriority.Valid() &&
		(input.Retention == nil || *input.Retention >= 0) &&
		(input.ApprovalLifetime == nil || *input.ApprovalLifetime > 0) &&
		workqueue.ValidatePolicyConfiguration(kind, input.Configuration) == nil
}

func policyChange(
	kind workqueue.Kind,
	targetID *string,
	input queuePolicyInput,
	actor string,
	at time.Time,
) workqueue.PolicyChange {
	return workqueue.PolicyChange{
		Kind: kind, TargetID: targetID, Enabled: input.Enabled,
		Cadence:   time.Duration(input.CadenceSeconds) * time.Second,
		ProfileID: input.ProfileID, DefaultPriority: input.DefaultPriority,
		RetryDelay: time.Duration(input.RetryDelay) * time.Second,
		Retention:  secondsPointer(input.Retention), ApprovalTTL: secondsPointer(input.ApprovalLifetime),
		Configuration: input.Configuration, ExpectedRevision: input.ExpectedRevision,
		ActorID: actor, ChangedAt: at,
	}
}

func secondsPointer(value *int64) *time.Duration {
	if value == nil {
		return nil
	}
	duration := time.Duration(*value) * time.Second

	return &duration
}

func (s *Server) getTargetSchedules(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	policies, err := s.policySet(r, target.ID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	profiles, err := s.store.ListScheduleProfiles(r.Context(), false)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	visible := make([]workqueue.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if profile.TargetID == nil || *profile.TargetID == target.ID {
			visible = append(visible, workspaceScheduleProfile(profile))
		}
	}
	statuses, err := s.store.ListQueuePolicyStatuses(r.Context(), &target.ID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"policies": policies, "profiles": visible, "statuses": statuses,
	})
}

func workspaceScheduleProfile(profile workqueue.Profile) workqueue.Profile {
	profile.AffectedWorkspaces = 0
	profile.AffectedItems = 0
	profile.AffectedPolicies = 0

	return profile
}

func (s *Server) policySet(r *http.Request, targetID string) (schedulePolicySet, error) {
	policies, err := s.store.ListQueuePolicies(r.Context(), &targetID)
	if err != nil {
		return schedulePolicySet{}, err
	}
	set := newSchedulePolicySet(s.deploymentQueuePolicies())
	for _, policy := range policies {
		if policy.TargetID == nil {
			set.Current = append(set.Current, policy)
		} else {
			set.Overrides = append(set.Overrides, policy)
		}
	}
	for _, kind := range workqueue.Kinds() {
		if kind == workqueue.KindScheduleChange {
			continue
		}
		policy, policyErr := s.store.GetEffectiveQueuePolicy(r.Context(), kind, &targetID)
		if policyErr != nil {
			return schedulePolicySet{}, policyErr
		}
		set.Effective = append(set.Effective, policy)
	}
	return set, nil
}

func (s *Server) deploymentQueuePolicies() []workqueue.Policy {
	return workqueue.DeploymentPolicies(workqueue.DeploymentDefaults{
		PollInterval:         s.cfg.PollInterval,
		PendingCIQuietPeriod: s.cfg.PendingCIQuietPeriod,
		PathIndexInterval:    s.cfg.PathIndexInterval,
	})
}

// scheduleRequestResponse carries the person who asked, not only their id.
//
// A request is one workspace asking an operator for something, and the panel
// prints it as a sentence with their name in it. The store holds the account id
// the request was made under, so the join happens here rather than in the page:
// a console that printed the id would be asking a reader to recognise one.
type scheduleRequestResponse struct {
	workqueue.ScheduleRequest

	Requester *accountResponse `json:"requester,omitempty"`
}

func (s *Server) scheduleRequestsDTO(
	r *http.Request,
	requests []workqueue.ScheduleRequest,
) []scheduleRequestResponse {
	// One lookup per person rather than per request, and an account that cannot
	// be read leaves the name absent rather than failing the page - the id is
	// still on the row, and a deleted asker is not a reason to refuse the list.
	people := make(map[string]*accountResponse, len(requests))
	decorated := make([]scheduleRequestResponse, 0, len(requests))
	for _, request := range requests {
		who, known := people[request.RequestedBy]
		if !known {
			if account, err := s.store.GetAccount(r.Context(), request.RequestedBy); err == nil {
				dto := accountDTO(account)
				who = &dto
			}
			people[request.RequestedBy] = who
		}
		decorated = append(decorated, scheduleRequestResponse{ScheduleRequest: request, Requester: who})
	}

	return decorated
}

func (s *Server) getRootScheduleRequests(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	requests, err := s.store.ListScheduleRequests(r.Context(), nil)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": s.scheduleRequestsDTO(r, requests)})
}

func (s *Server) getTargetScheduleRequests(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	requests, err := s.store.ListScheduleRequests(r.Context(), &target.ID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": s.scheduleRequestsDTO(r, requests)})
}

func (s *Server) postTargetScheduleRequest(w http.ResponseWriter, r *http.Request) {
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
	var input scheduleRequestInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validScheduleRequestInput(input) {
		s.writeError(w, http.StatusBadRequest, "invalid_schedule_request", "schedule request is invalid")
		return
	}
	id, err := randomToken(s.random)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	request, err := s.store.CreateScheduleRequest(r.Context(), workqueue.ScheduleRequestCreate{
		ID: "request:" + id, TargetID: target.ID, Kind: input.Kind,
		BaseRevision: input.BaseRevision, ProfileID: input.ProfileID,
		CustomProfile:   input.CustomProfile,
		Cadence:         time.Duration(input.CadenceSeconds) * time.Second,
		DefaultPriority: input.DefaultPriority, Configuration: input.Configuration,
		Reason: strings.TrimSpace(input.Reason), RequestedBy: account.ID, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: target.ID})
	writeJSON(w, http.StatusCreated, request)
}

func validScheduleRequestInput(input scheduleRequestInput) bool {
	return input.Kind.WorkspaceConfigurable() &&
		input.CadenceSeconds >= 0 && (!input.Kind.Recurring() || input.CadenceSeconds > 0) &&
		input.DefaultPriority.Valid() &&
		strings.TrimSpace(input.Reason) != "" &&
		(input.ProfileID == nil) != (input.CustomProfile == nil) &&
		workqueue.ValidatePolicyConfiguration(input.Kind, input.Configuration) == nil
}

func (s *Server) postRootScheduleDecision(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	var input scheduleDecisionInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.ExpectedRevision == nil || strings.TrimSpace(input.Reason) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_decision", "revision and decision reason are required")
		return
	}
	request, err := s.store.DecideScheduleRequest(r.Context(), r.PathValue("request"), workqueue.ScheduleDecision{
		Approve: input.Approve, PromoteProfile: input.PromoteProfile,
		ProfileID: input.ProfileID, DecisionReason: strings.TrimSpace(input.Reason),
		ExpectedRevision: *input.ExpectedRevision, ReviewerID: account.ID, ReviewedAt: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: request.TargetID})
	s.wakeScheduledWork(request.Kind.Lane())
	writeJSON(w, http.StatusOK, request)
}

func (s *Server) wakeScheduledWork(lane workqueue.Lane) {
	if s.queue != nil {
		s.queue.WakeQueue(lane)
	}
	if lane == workqueue.LanePendingCI && s.pendingCI != nil {
		s.pendingCI.Wake()
	}
}

func (s *Server) deleteTargetScheduleRequest(w http.ResponseWriter, r *http.Request) {
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
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	id := r.PathValue("request")
	current, err := s.store.GetScheduleRequest(r.Context(), id)
	if err != nil || current.TargetID != target.ID {
		if err == nil {
			err = storage.ErrNotFound
		}
		s.writeStorageError(w, err)
		return
	}
	request, err := s.store.WithdrawScheduleRequest(
		r.Context(), id, revision, account.ID, s.now().UTC(),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: target.ID})
	writeJSON(w, http.StatusOK, request)
}

func requiredRevision(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.URL.Query().Get("expected_revision")
	revision, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "expected_revision is required")
		return 0, false
	}

	return revision, true
}
