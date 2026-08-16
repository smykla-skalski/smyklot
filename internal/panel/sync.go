package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// syncConfigRequest is a saved label configuration.
//
// Revision is required rather than optional. Two people editing the same label
// set from two tabs is the ordinary case, and a write with no opinion about
// what it is replacing is a write that silently wins.
type syncConfigRequest struct {
	Enabled          *bool           `json:"enabled"`
	Labels           []orgsync.Label `json:"labels"`
	AllowRemoval     bool            `json:"allow_removal"`
	Excludes         []string        `json:"excludes"`
	ExpectedRevision *int64          `json:"expected_revision"`
}

// syncConfigDTO is what the panel reads back.
type syncConfigDTO struct {
	Kind         string          `json:"kind"`
	Enabled      bool            `json:"enabled"`
	Labels       []orgsync.Label `json:"labels"`
	AllowRemoval bool            `json:"allow_removal"`
	Excludes     []string        `json:"excludes"`
	Revision     int64           `json:"revision"`
	UpdatedBy    string          `json:"updated_by"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Digest       string          `json:"digest"`
}

// syncPlanDTO is a plan as a person reads it: what it would do, and enough to
// approve exactly this one.
type syncPlanDTO struct {
	ID       string          `json:"id"`
	Trigger  string          `json:"trigger"`
	State    string          `json:"state"`
	Digest   string          `json:"digest"`
	Counts   syncCountsDTO   `json:"counts"`
	Actions  []syncActionDTO `json:"actions"`
	Computed time.Time       `json:"computed_at"`
	Expires  time.Time       `json:"expires_at"`
	Approved *time.Time      `json:"approved_at,omitempty"`
	Finished *time.Time      `json:"finished_at,omitempty"`
}

type syncCountsDTO struct {
	Create int `json:"create"`
	Update int `json:"update"`
	Delete int `json:"delete"`
}

type syncActionDTO struct {
	Repository string `json:"repository"`
	Kind       string `json:"kind"`
	Operation  string `json:"operation"`
	Subject    string `json:"subject"`
	Before     string `json:"before,omitempty"`
	After      string `json:"after,omitempty"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	Blocker    string `json:"blocker,omitempty"`
}

// getSyncConfig reads an installation's sync configuration.
func (s *Server) getSyncConfig(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}

	config, err := s.store.GetSyncConfig(r.Context(), target.ID, orgsync.KindLabels)
	if errors.Is(err, storage.ErrNotFound) {
		// Never configured, which is not an error and not the same as
		// configured and switched off. An empty answer says so.
		writeJSON(w, http.StatusOK, syncConfigDTO{
			Kind: string(orgsync.KindLabels), Labels: []orgsync.Label{}, Excludes: []string{},
		})

		return
	}
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, syncConfigToDTO(config))
}

// putSyncConfig saves it.
//
// Validated here rather than at apply time, because every rule it checks is one
// GitHub would answer with a 422 that abandons the rest of that repository's
// labels. Answering now puts the message beside the field somebody typed.
func (s *Server) putSyncConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}

	var input syncConfigRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Enabled == nil || input.ExpectedRevision == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request",
			"a sync configuration needs to say whether it is enabled and what it replaces")

		return
	}

	config := orgsync.LabelConfig{Labels: input.Labels, AllowRemoval: input.AllowRemoval}
	if err := config.Validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_sync_config", err.Error())

		return
	}
	if err := (orgsync.Excludes{Patterns: input.Excludes}).Validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_sync_config", err.Error())

		return
	}

	document, err := json.Marshal(syncDocument{
		Labels: config.Labels, AllowRemoval: config.AllowRemoval, Excludes: input.Excludes,
	})
	if err != nil {
		s.writeInternal(w, err)

		return
	}

	saved, err := s.store.SetSyncConfig(r.Context(), orgsync.ConfigChange{
		TargetID: target.ID,
		Kind:     orgsync.KindLabels,
		Enabled:  *input.Enabled,
		Document: document,
		ActorID:  account.ID,
		Now:      s.now().UTC(),
		Revision: *input.ExpectedRevision,
	})
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	s.Announce(target.ID, "")
	writeJSON(w, http.StatusOK, syncConfigToDTO(saved))
}

// syncPlanKey is the JSON key a plan arrives under, and the wildcard the route
// names it by.
const syncPlanKey = "plan"

// syncDocument is the stored shape of a label configuration.
//
// Its own type rather than orgsync.LabelConfig, because the excludes travel
// with it and the planner takes them separately. One document, one decode.
type syncDocument struct {
	Labels       []orgsync.Label `json:"labels"`
	AllowRemoval bool            `json:"allow_removal"`
	Excludes     []string        `json:"excludes"`
}

func syncConfigToDTO(config orgsync.Config) syncConfigDTO {
	dto := syncConfigDTO{
		Kind:      string(config.Kind),
		Enabled:   config.Enabled,
		Labels:    []orgsync.Label{},
		Excludes:  []string{},
		Revision:  config.Revision,
		UpdatedBy: config.UpdatedBy,
		UpdatedAt: config.UpdatedAt,
		Digest:    config.Digest,
	}

	var document syncDocument
	if err := json.Unmarshal(config.Document, &document); err != nil {
		// A stored document that will not decode is shown as empty rather than
		// failing the page. The row is what it is, and a panel that will not
		// render is a panel nobody can use to fix it.
		return dto
	}

	if document.Labels != nil {
		dto.Labels = document.Labels
	}
	if document.Excludes != nil {
		dto.Excludes = document.Excludes
	}
	dto.AllowRemoval = document.AllowRemoval

	return dto
}

// getSyncPlan reads whatever plan an installation has in flight.
func (s *Server) getSyncPlan(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}

	plan, actions, err := s.store.GetLiveSyncPlan(r.Context(), target.ID)
	if errors.Is(err, storage.ErrNotFound) {
		// Nothing in flight is an answer, not a missing page.
		writeJSON(w, http.StatusOK, map[string]any{syncPlanKey: nil})

		return
	}
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{syncPlanKey: syncPlanToDTO(plan, actions)})
}

// postSyncPlanApproval accepts a plan somebody has read.
func (s *Server) postSyncPlanApproval(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}

	var input struct {
		Digest string `json:"digest"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Digest == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request",
			"an approval has to say which plan it is approving")

		return
	}

	plan, err := s.store.ApproveSyncPlan(r.Context(), orgsync.PlanApproval{
		PlanID:  r.PathValue(syncPlanKey),
		Digest:  input.Digest,
		ActorID: account.ID,
		Now:     s.now().UTC(),
	})
	if errors.Is(err, orgsync.ErrStalePlan) {
		// The one refusal worth its own message: what is on the screen is not
		// what is in the database, so approving it would apply something else.
		s.writeError(w, http.StatusConflict, "stale_plan",
			"this plan no longer matches the configuration; ask for a new one")

		return
	}
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	if err := s.store.RecordSyncAudit(r.Context(), orgsync.AuditEntry{
		TargetID: target.ID, PlanID: plan.ID, ActorID: account.ID,
		Action:  orgsync.AuditApproved,
		Summary: syncApprovalSummary(plan.Counts),
		Counts:  plan.Counts,
		Now:     s.now().UTC(),
	}); err != nil {
		s.writeInternal(w, err)

		return
	}

	s.Announce(target.ID, "")

	_, actions, err := s.store.GetSyncPlan(r.Context(), plan.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{syncPlanKey: syncPlanToDTO(plan, actions)})
}

func syncApprovalSummary(counts orgsync.Counts) string {
	return strconv.Itoa(counts.Total()) + " changes approved"
}

func syncPlanToDTO(plan orgsync.Plan, actions []orgsync.Action) syncPlanDTO {
	dto := syncPlanDTO{
		ID:      plan.ID,
		Trigger: string(plan.Trigger),
		State:   string(plan.State),
		Digest:  plan.Digest,
		Counts: syncCountsDTO{
			Create: plan.Counts.Create,
			Update: plan.Counts.Update,
			Delete: plan.Counts.Delete,
		},
		Actions:  make([]syncActionDTO, 0, len(actions)),
		Computed: plan.ComputedAt,
		Expires:  plan.ExpiresAt,
		Approved: plan.ApprovedAt,
		Finished: plan.FinishedAt,
	}

	for _, action := range actions {
		dto.Actions = append(dto.Actions, syncActionDTO{
			Repository: action.RepositoryID,
			Kind:       string(action.Kind),
			Operation:  string(action.Operation),
			Subject:    action.Subject,
			Before:     action.Before,
			After:      action.After,
			State:      string(action.State),
			Error:      action.Error,
			Blocker:    string(action.Blocker),
		})
	}

	return dto
}
