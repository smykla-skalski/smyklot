package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

// syncConfigRequest carries one kind's fields into the shared settings batch
// validator. Labels use typed fields; other kinds pass their document through.
type syncConfigRequest struct {
	Labels       []orgsync.Label `json:"labels"`
	AllowRemoval bool            `json:"allow_removal"`
	Excludes     []string        `json:"excludes"`

	// Document is a kind whose shape the panel has no form for, sent through
	// untouched. Labels use the typed fields above; everything else arrives
	// here, so a new kind is configurable before it has a form.
	Document json.RawMessage `json:"document,omitempty"`
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

	// Document is the stored configuration as it is, whatever kind it belongs
	// to.
	//
	// The typed fields above describe labels, which is the kind the panel has a
	// form for. Every other kind travels here untouched, so adding one needs no
	// change on this side - and the fields it does not have come back as an
	// empty object rather than as null, which a browser would have to guard.
	Document json.RawMessage `json:"document"`

	// Unreadable is a stored document this version cannot decode. The lists
	// above are then empty because nothing could be read out of them, not
	// because the installation configured nothing - and a panel that could not
	// tell those apart would offer an empty form somebody saves, wiping a label
	// set that was never shown to them.
	Unreadable bool `json:"unreadable"`

	// Unavailable says what this kind needs and the installation has not
	// granted, or is empty.
	//
	// Saving before the permission is approved is the ordinary order during a
	// rollout, so nothing here refuses the save. What it prevents is the answer
	// afterwards: a kind switched on that the installation cannot act on plans
	// nothing and fails at nothing, and an empty plan list reads exactly like a
	// sweep that has not come round yet. Settings sync is the first kind to
	// need a permission no existing installation has, which makes this the
	// ordinary first-use answer rather than a corner of one.
	Unavailable string `json:"unavailable"`
}

// emptyDocument is what a kind nobody has configured answers with. An object
// rather than null, so a reader has one shape to handle.
var emptyDocument = json.RawMessage(`{}`)

// syncPlanDTO is a plan as a person reads it: what it would do, and enough to
// approve exactly this one.
type syncPlanDTO struct {
	ID             string          `json:"id"`
	Trigger        string          `json:"trigger"`
	State          string          `json:"state"`
	Digest         string          `json:"digest"`
	Counts         syncCountsDTO   `json:"counts"`
	Actions        []syncActionDTO `json:"actions"`
	Computed       time.Time       `json:"computed_at"`
	Expires        time.Time       `json:"expires_at"`
	Approved       *time.Time      `json:"approved_at,omitempty"`
	Finished       *time.Time      `json:"finished_at,omitempty"`
	ExecutionStage string          `json:"execution_stage"`
	Queue          *workqueue.Item `json:"queue_item,omitempty"`
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

type syncRunNowInput struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type syncRunNowResponse struct {
	Status string          `json:"status"`
	Plan   *syncPlanDTO    `json:"plan,omitempty"`
	Queue  *workqueue.Item `json:"queue_item,omitempty"`
}

// syncKind reads the kind from the address, refusing one this version does not
// know rather than answering about a kind nothing can sync.
func (s *Server) syncKind(w http.ResponseWriter, r *http.Request) (orgsync.Kind, bool) {
	kind := orgsync.Kind(r.PathValue(syncKindKey))
	if !kind.Valid() {
		s.writeError(w, http.StatusNotFound, "unknown_sync_kind",
			"Smyklot does not synchronize that")

		return "", false
	}

	return kind, true
}

// syncKindKey is the wildcard the sync routes name a kind by.
const syncKindKey = "kind"

// getSyncConfig reads an installation's sync configuration for one kind.
func (s *Server) getSyncConfig(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	kind, ok := s.syncKind(w, r)
	if !ok {
		return
	}

	config, err := s.store.GetSyncConfig(r.Context(), target.ID, kind)
	if errors.Is(err, storage.ErrNotFound) {
		// Never configured, which is not an error and not the same as
		// configured and switched off. A configuration with nothing in it says
		// so, through the same answer as any other, so what a browser reads of
		// a kind nobody has touched has one shape rather than two.
		config = orgsync.Config{Kind: kind}
	} else if err != nil {
		s.writeStorageError(w, err)

		return
	}

	updatedBy, err := s.syncEditorLogin(r.Context(), config.UpdatedBy)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, syncConfigAnswer(config, target, updatedBy))
}

// syncPlanKey is the JSON key a plan arrives under, and the wildcard the route
// names it by.
const syncPlanKey = "plan"

// syncDocumentFor validates a kind's configuration and returns what to store.
//
// The stored document is the domain type itself, so what the planner reads is
// what the panel wrote. A second shape here is what made every configured
// exclusion a silent no-op in the kind before this one: the planner decoded the
// type without them.
//
// Validated now rather than at apply time, because every rule checked is one
// GitHub answers with a 422 that abandons the rest of that repository's change.
// Answering here puts the message beside the field somebody typed.
func syncDocumentFor(kind orgsync.Kind, input syncConfigRequest) ([]byte, error) {
	switch kind {
	case orgsync.KindLabels:
		config := orgsync.LabelConfig{
			Labels:       input.Labels,
			AllowRemoval: input.AllowRemoval,
			Excludes:     input.Excludes,
		}
		if err := config.Validate(); err != nil {
			return nil, err
		}

		return json.Marshal(config)

	default:
		return validatedSyncDocument(kind, input.Document)
	}
}

// validatedSyncDocument applies the current strict schema and domain rules to
// a stored or incoming document, and returns its canonical representation.
func validatedSyncDocument(kind orgsync.Kind, document json.RawMessage) ([]byte, error) {
	switch kind {
	case orgsync.KindLabels:
		return validatedDocument[orgsync.LabelConfig](document)
	case orgsync.KindSettings:
		return validatedDocument[orgsync.SettingsConfig](document)
	case orgsync.KindRulesets:
		return validatedDocument[orgsync.RulesetConfig](document)
	case orgsync.KindFiles:
		return validatedDocument[orgsync.FileConfig](document)
	default:
		return nil, fmt.Errorf("%w: Smyklot cannot synchronize %s yet",
			orgsync.ErrInvalidConfig, kind)
	}
}

// validatedDocument reads a kind whose whole configuration travels as one
// document, checks it, and answers what to store.
//
// Labels are the exception rather than the rule here: they arrive as typed
// fields on the request because the form was built before there was a second
// kind. Everything since carries a document, so this is the shape a new kind
// takes.
func validatedDocument[T interface{ Validate() error }](
	document json.RawMessage,
) ([]byte, error) {
	var config T
	if err := decodeStrictly(document, &config); err != nil {
		return nil, err
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return json.Marshal(config)
}

// decodeStrictly reads a kind's document, refusing a key this version does not
// know.
//
// The panel is where a human wrote it, and a setting spelled wrong that is
// quietly dropped is a configuration somebody believes is in force - which is
// why an unknown key in SMYKLOT_CONFIG became a hard error rather than a shrug.
// The planner reads the same document leniently, and deliberately: by then
// nobody can fix it, and a key a newer version wrote must not take a kind down
// through a rollback.
func decodeStrictly(document json.RawMessage, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(documentOrEmpty(document)))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("%w: %w", orgsync.ErrInvalidConfig, err)
	}

	return nil
}

func documentOrEmpty(document json.RawMessage) json.RawMessage {
	if len(document) == 0 {
		return emptyDocument
	}

	return document
}

// readableDocument answers what to carry out and whether the stored bytes could
// be read at all.
//
// Checked before they are carried anywhere. A json.RawMessage is validated as
// it is copied out, so holding one that does not parse fails the whole response
// rather than this one field - and a row this version cannot decode has to
// render as itself rather than as a configuration that adjusts nothing, which
// is what somebody would then save over.
func readableDocument(document json.RawMessage) (json.RawMessage, bool) {
	if !json.Valid(document) {
		return emptyDocument, false
	}

	return document, true
}

// syncConfigAnswer is one kind's configuration together with whether the
// installation may act on it.
//
// Two questions in one answer because they are read as one: a switch that is on
// means nothing without the permission behind it, and the panel asks for the
// configuration at exactly the moment somebody wants to know why nothing is
// happening.
func syncConfigAnswer(
	config orgsync.Config,
	target storage.Target,
	updatedBy string,
) syncConfigDTO {
	dto := syncConfigToDTO(config, updatedBy)
	if unavailable, missing := orgsync.UnpermittedConfig(target, config); missing {
		dto.Unavailable = unavailable.Reason()
	}

	return dto
}

func syncConfigToDTO(config orgsync.Config, updatedBy string) syncConfigDTO {
	dto := syncConfigDTO{
		Kind:      string(config.Kind),
		Enabled:   config.Enabled,
		Labels:    []orgsync.Label{},
		Excludes:  []string{},
		Revision:  config.Revision,
		UpdatedBy: updatedBy,
		UpdatedAt: config.UpdatedAt,
		Digest:    config.Digest,
		Document:  documentOrEmpty(config.Document),
	}

	// The row is still there; nothing here pretends to have read it, and the
	// answer somebody would otherwise get is a truncated body rather than the
	// message saying what is wrong.
	carried, readable := readableDocument(dto.Document)
	dto.Document, dto.Unreadable = carried, !readable

	if !readable {
		return dto
	}

	if config.Kind != orgsync.KindLabels {
		// Only labels have typed fields here. Every other kind travels in
		// Document, which is already set, and inventing empty label lists for
		// it would describe a configuration it does not have.
		return dto
	}

	// What was checked just above, rather than what it was read from: a kind
	// nobody has configured holds no document at all, and decoding those bytes
	// would report a row that does not exist as one this version cannot read.
	var document orgsync.LabelConfig
	if err := json.Unmarshal(dto.Document, &document); err != nil {
		// The page still renders - a panel that will not load is a panel nobody
		// can use to fix anything - but it says so, and it says so because the
		// alternative is worse than a blank screen. An empty list that looks
		// like a configuration is one somebody saves over, and the save would
		// send back the emptiness the panel invented rather than the labels the
		// row still holds.
		dto.Unreadable = true

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

// syncEditorLogin turns the stable account key stored with a change into the
// current GitHub login a person can recognize. The storage key remains the
// provenance and plan actor; only the panel boundary replaces it with display
// data.
func (s *Server) syncEditorLogin(ctx context.Context, accountID string) (string, error) {
	if accountID == "" {
		return "", nil
	}

	account, err := s.store.GetAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("read sync editor: %w", err)
	}

	return account.Login, nil
}

// getSyncPlan reads whatever plan an installation has in flight.
func (s *Server) getSyncPlan(w http.ResponseWriter, r *http.Request) {
	_, target, access, ok := s.requireTarget(w, r, false)
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

	repositoryNames, err := s.syncPlanRepositoryNames(r.Context(), target.ID, actions)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}
	dto := syncPlanToDTO(plan, actions, repositoryNames)
	if err := s.attachSyncPlanQueue(r.Context(), &dto, access.Role); err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{syncPlanKey: dto})
}

func (s *Server) postSyncRunNow(w http.ResponseWriter, r *http.Request) {
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
	var input syncRunNowInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "run now requires a reason")
		return
	}
	plan, actions, err := s.store.GetLiveSyncPlan(r.Context(), target.ID)
	if err == nil {
		s.handleLiveSyncRunNow(w, r, account, target, access.Role, input, plan, actions)
		return
	}
	if !errors.Is(err, storage.ErrNotFound) {
		s.writeStorageError(w, err)
		return
	}
	item, err := s.store.RequestRecurringWork(r.Context(), workqueue.RecurringRequest{
		Kind: workqueue.KindSyncScan, TargetID: &target.ID,
		Title: "Scan organization sync drift", ActorID: account.ID,
		Reason: input.Reason, Now: s.now().UTC(),
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	prepareQueueItem(&item, true, false)
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: target.ID})
	s.wakeScheduledWork(workqueue.LaneMaintenance)
	writeJSON(w, http.StatusAccepted, syncRunNowResponse{Status: "scan_queued", Queue: &item})
}

func (s *Server) handleLiveSyncRunNow(
	w http.ResponseWriter,
	r *http.Request,
	account storage.Account,
	target storage.Target,
	role storage.InstallationRole,
	input syncRunNowInput,
	plan orgsync.Plan,
	actions []orgsync.Action,
) {
	dto, err := s.syncPlanDTO(r.Context(), plan, actions, role)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	switch plan.State {
	case orgsync.PlanComputed:
		writeJSON(w, http.StatusOK, syncRunNowResponse{Status: "approval_required", Plan: &dto})
	case orgsync.PlanApplying:
		writeJSON(w, http.StatusOK, syncRunNowResponse{Status: "already_running", Plan: &dto})
	case orgsync.PlanApproved:
		if dto.Queue == nil || input.ExpectedRevision != dto.Queue.Revision {
			writeJSON(w, http.StatusConflict, map[string]any{
				jsonFieldCode: errorCodeStaleRevision, jsonFieldMessage: "sync queue item changed; review the latest state",
				jsonFieldCurrent: dto,
			})
			return
		}
		item, actionErr := s.store.ApplyQueueAction(r.Context(), dto.Queue.ID, workqueue.ItemAction{
			Type: workqueue.ActionRunNow, ExpectedRevision: input.ExpectedRevision,
			ActorID: account.ID, Reason: input.Reason, ChangedAt: s.now().UTC(),
		})
		if actionErr != nil {
			s.writeStorageError(w, actionErr)
			return
		}
		prepareQueueItem(&item, true, false)
		dto.Queue = &item
		s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: target.ID})
		s.wakeScheduledWork(workqueue.LaneMaintenance)
		writeJSON(w, http.StatusAccepted, syncRunNowResponse{
			Status: "plan_dispatched", Plan: &dto, Queue: &item,
		})
	default:
		s.writeError(w, http.StatusConflict, "unsupported_plan_state", "sync plan cannot run now")
	}
}

func (s *Server) syncPlanDTO(
	ctx context.Context,
	plan orgsync.Plan,
	actions []orgsync.Action,
	role storage.InstallationRole,
) (syncPlanDTO, error) {
	repositoryNames, err := s.syncPlanRepositoryNames(ctx, plan.TargetID, actions)
	if err != nil {
		return syncPlanDTO{}, err
	}
	dto := syncPlanToDTO(plan, actions, repositoryNames)
	if err := s.attachSyncPlanQueue(ctx, &dto, role); err != nil {
		return syncPlanDTO{}, err
	}

	return dto, nil
}

// postSyncPlanApproval accepts a plan somebody has read.
func (s *Server) postSyncPlanApproval(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, access, ok := s.requireTarget(w, r, true)
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
		TargetID: target.ID,
		PlanID:   r.PathValue(syncPlanKey),
		Digest:   input.Digest,
		ActorID:  account.ID,
		Now:      s.now().UTC(),
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
	s.wakeScheduledWork(workqueue.LaneMaintenance)

	_, actions, err := s.store.GetSyncPlan(r.Context(), target.ID, plan.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}
	repositoryNames, err := s.syncPlanRepositoryNames(r.Context(), target.ID, actions)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	dto := syncPlanToDTO(plan, actions, repositoryNames)
	if err := s.attachSyncPlanQueue(r.Context(), &dto, access.Role); err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{syncPlanKey: dto})
}

func (s *Server) attachSyncPlanQueue(
	ctx context.Context,
	dto *syncPlanDTO,
	role storage.InstallationRole,
) error {
	item, err := s.store.GetQueueItem(ctx, "sync-plan:"+dto.ID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	canControl := role == storage.InstallationRoleAdmin || role == storage.InstallationRoleOwner
	prepareQueueItem(&item, canControl, false)
	dto.Queue = &item

	return nil
}

func syncApprovalSummary(counts orgsync.Counts) string {
	return strconv.Itoa(counts.Total()) + " changes approved"
}

func (s *Server) syncPlanRepositoryNames(
	ctx context.Context,
	targetID string,
	actions []orgsync.Action,
) (map[string]string, error) {
	names := make(map[string]string)
	for _, action := range actions {
		if _, found := names[action.RepositoryID]; found {
			continue
		}
		repository, err := s.store.GetRepository(ctx, targetID, action.RepositoryID)
		if err != nil {
			return nil, fmt.Errorf("read sync plan repository: %w", err)
		}
		names[repository.ID] = repository.Name
	}

	return names, nil
}

func syncPlanToDTO(
	plan orgsync.Plan,
	actions []orgsync.Action,
	repositoryNames map[string]string,
) syncPlanDTO {
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
		Actions:        make([]syncActionDTO, 0, len(actions)),
		Computed:       plan.ComputedAt,
		Expires:        plan.ExpiresAt,
		Approved:       plan.ApprovedAt,
		Finished:       plan.FinishedAt,
		ExecutionStage: syncExecutionStage(plan, actions),
	}

	for _, action := range actions {
		repository := repositoryNames[action.RepositoryID]
		if repository == "" {
			repository = action.RepositoryID
		}
		dto.Actions = append(dto.Actions, syncActionDTO{
			Repository: repository,
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

func syncExecutionStage(plan orgsync.Plan, actions []orgsync.Action) string {
	total, settled := len(actions), 0
	for _, action := range actions {
		if action.State != orgsync.ActionPending {
			settled++
		}
	}
	switch plan.State {
	case orgsync.PlanComputed:
		return "Waiting for approval"
	case orgsync.PlanApproved:
		return "Waiting for scheduled dispatch"
	case orgsync.PlanApplying:
		return fmt.Sprintf("Applying changes - %d of %d settled", settled, total)
	case orgsync.PlanApplied:
		return "All changes applied"
	case orgsync.PlanFailed:
		return "Execution finished with failures"
	case orgsync.PlanStale:
		return "A fresh drift scan is required"
	case orgsync.PlanExpired:
		return "Approval expired - a fresh drift scan is required"
	default:
		return string(plan.State)
	}
}
