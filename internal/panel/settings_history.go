package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const maxWorkspaceSettingsRestoreBody = 1 << 20

type workspaceSettingsRestoreRequest struct {
	State      string                                     `json:"state"`
	Selections []workspaceSettingsRestoreSelectionRequest `json:"selections"`
}

type workspaceSettingsRestoreSelectionRequest struct {
	Kind             string `json:"kind"`
	RepositoryID     string `json:"repository_id,omitempty"`
	SyncKind         string `json:"sync_kind,omitempty"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

type settingsCheckpointResponse struct {
	ID             string                                `json:"id"`
	Action         string                                `json:"action"`
	Actor          accountResponse                       `json:"actor"`
	RestoredFromID *string                               `json:"restored_from_id,omitempty"`
	RestoredSide   storage.SettingsCheckpointRestoreSide `json:"restored_side,omitempty"`
	CreatedAt      time.Time                             `json:"created_at"`
	AffectedKinds  []storage.SettingsCheckpointItemKind  `json:"affected_kinds"`
	Items          []settingsCheckpointItemResponse      `json:"items"`
}

type settingsCheckpointItemResponse struct {
	Kind               storage.SettingsCheckpointItemKind `json:"kind"`
	RepositoryID       string                             `json:"repository_id,omitempty"`
	RepositoryFullName string                             `json:"repository_full_name,omitempty"`
	SyncKind           orgsync.Kind                       `json:"sync_kind,omitempty"`
	DocumentVersion    int                                `json:"document_version"`
	Before             settingsCheckpointSideResponse     `json:"before"`
	After              settingsCheckpointSideResponse     `json:"after"`
	Current            *settingsCheckpointState           `json:"current"`
	Changed            bool                               `json:"changed"`
}

type settingsCheckpointSideResponse struct {
	Available       bool                               `json:"available"`
	State           *settingsCheckpointState           `json:"state"`
	Differs         bool                               `json:"differs"`
	Restorable      bool                               `json:"restorable"`
	Incompatibility *settingsCheckpointIncompatibility `json:"incompatibility,omitempty"`
}

type settingsCheckpointState struct {
	Document json.RawMessage `json:"document"`
	Digest   string          `json:"digest"`
	Revision int64           `json:"revision"`
}

type settingsCheckpointIncompatibility struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type workspaceSettingsRestoreActor struct {
	accountID        string
	elevationID      *string
	sessionTokenHash string
	root             bool
	writeError       func(http.ResponseWriter, error)
}

func (s *Server) getWorkspaceSettingsCheckpoint(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	s.writeWorkspaceSettingsCheckpoint(w, r, target)
}

func (s *Server) getRootWorkspaceSettingsCheckpoint(w http.ResponseWriter, r *http.Request) {
	root, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	s.writeWorkspaceSettingsCheckpoint(w, r, root.Target)
}

func (s *Server) getWorkspaceSettingsBaseline(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	s.writeWorkspaceSettingsBaseline(w, r, target)
}

func (s *Server) getRootWorkspaceSettingsBaseline(w http.ResponseWriter, r *http.Request) {
	root, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	s.writeWorkspaceSettingsBaseline(w, r, root.Target)
}

func (s *Server) writeWorkspaceSettingsBaseline(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
) {
	inspection, err := s.store.InspectInstallationSettingsBaseline(r.Context(), target.ID)
	if err != nil {
		s.writeSettingsHistoryError(w, err, s.writeStorageError)
		return
	}
	s.writeSettingsCheckpointInspection(w, r, inspection)
}

func (s *Server) writeWorkspaceSettingsCheckpoint(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
) {
	checkpointID, ok := s.settingsCheckpointID(w, r)
	if !ok {
		return
	}
	inspection, err := s.store.InspectInstallationSettingsCheckpoint(
		r.Context(), storage.SettingsCheckpointRef{
			ID: checkpointID, Scope: storage.SettingsCheckpointScopeInstallation,
			TargetID: target.ID,
		},
	)
	if err != nil {
		s.writeSettingsHistoryError(w, err, s.writeStorageError)
		return
	}
	s.writeSettingsCheckpointInspection(w, r, inspection)
}

func (s *Server) writeSettingsCheckpointInspection(
	w http.ResponseWriter,
	r *http.Request,
	inspection storage.SettingsCheckpointInspection,
) {
	actor, err := s.store.GetAccount(r.Context(), inspection.Checkpoint.ActorAccountID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsCheckpointDTO(inspection, actor))
}

func (s *Server) postWorkspaceSettingsRestore(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}
	s.restoreWorkspaceSettingsCheckpoint(w, r, target, workspaceSettingsRestoreActor{
		accountID: account.ID, writeError: s.writeStorageError,
	})
}

func (s *Server) postRootWorkspaceSettingsRestore(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	root, ok := s.requireRootTarget(w, r, true)
	if !ok {
		return
	}
	s.restoreWorkspaceSettingsCheckpoint(w, r, root.Target, workspaceSettingsRestoreActor{
		accountID: root.Account.ID, elevationID: elevationID(root.Elevation),
		sessionTokenHash: root.SessionHash, root: true, writeError: s.writeRootWriteError,
	})
}

func (s *Server) restoreWorkspaceSettingsCheckpoint(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
	actor workspaceSettingsRestoreActor,
) {
	request, ok := s.prepareWorkspaceSettingsRestore(w, r, target.ID, actor)
	if !ok {
		return
	}
	result, err := s.restoreWorkspaceSettings(r.Context(), request)
	if err != nil {
		s.writeWorkspaceSettingsRestoreError(w, err, actor)
		return
	}
	s.signalWorkspaceSettingsBatch(target.ID, result)
	writeJSON(w, http.StatusOK, workspaceSettingsRestoreAnswer(target.ID, result))
}

func (s *Server) writeWorkspaceSettingsRestoreError(
	w http.ResponseWriter,
	err error,
	actor workspaceSettingsRestoreActor,
) {
	if actor.root && (errors.Is(err, storage.ErrNotFound) ||
		errors.Is(err, storage.ErrConflict) ||
		errors.Is(err, storage.ErrExpired) ||
		errors.Is(err, storage.ErrRevoked)) {
		actor.writeError(w, err)
		return
	}

	s.writeSettingsHistoryError(w, err, actor.writeError)
}

func (s *Server) prepareWorkspaceSettingsRestore(
	w http.ResponseWriter,
	r *http.Request,
	targetID string,
	actor workspaceSettingsRestoreActor,
) (storage.RestoreInstallationSettingsRequest, bool) {
	checkpointID, ok := s.settingsCheckpointID(w, r)
	if !ok {
		return storage.RestoreInstallationSettingsRequest{}, false
	}
	var input workspaceSettingsRestoreRequest
	if !decodeJSONWithin(w, r, &input, maxWorkspaceSettingsRestoreBody) {
		return storage.RestoreInstallationSettingsRequest{}, false
	}
	request := storage.RestoreInstallationSettingsRequest{
		TargetID: targetID, CheckpointID: checkpointID, ActorAccountID: actor.accountID,
		ElevationID: actor.elevationID, SessionTokenHash: actor.sessionTokenHash,
		ChangedAt: s.now().UTC(), DeploymentPendingCIQuietPeriod: s.cfg.PendingCIQuietPeriod,
		Side:       storage.SettingsCheckpointRestoreSide(input.State),
		Selections: make([]storage.SettingsCheckpointRestoreSelection, 0, len(input.Selections)),
	}
	for _, inputSelection := range input.Selections {
		if inputSelection.ExpectedRevision == nil {
			s.writeInvalidSettingsRestore(w)
			return storage.RestoreInstallationSettingsRequest{}, false
		}
		request.Selections = append(request.Selections, storage.SettingsCheckpointRestoreSelection{
			Identity: storage.SettingsCheckpointItemIdentity{
				Kind:         storage.SettingsCheckpointItemKind(inputSelection.Kind),
				RepositoryID: inputSelection.RepositoryID,
				SyncKind:     orgsync.Kind(inputSelection.SyncKind),
			},
			ExpectedRevision: *inputSelection.ExpectedRevision,
		})
	}
	if err := request.Validate(); err != nil {
		s.writeInvalidSettingsRestore(w)
		return storage.RestoreInstallationSettingsRequest{}, false
	}

	return request, true
}

func (s *Server) writeInvalidSettingsRestore(w http.ResponseWriter) {
	s.writeError(w, http.StatusBadRequest, "invalid_request",
		"each selected setting must be known, unique, and name its current revision")
}

func (s *Server) restoreWorkspaceSettings(
	ctx context.Context,
	request storage.RestoreInstallationSettingsRequest,
) (storage.SaveInstallationSettingsResult, error) {
	operation := func() (storage.SaveInstallationSettingsResult, error) {
		return s.store.RestoreInstallationSettings(ctx, request)
	}
	if settingsRestoreIncludesTarget(request.Selections) {
		return s.saveWorkspaceTargetSettingsBatch(ctx, request.TargetID, operation)
	}
	repositoryIDs := settingsRestoreRepositoryIDs(request.Selections)
	if len(repositoryIDs) == 0 {
		return operation()
	}

	return saveWorkspaceSettingsExclusive(s, ctx, repositoryIDs, operation)
}

func settingsRestoreIncludesTarget(
	selections []storage.SettingsCheckpointRestoreSelection,
) bool {
	for _, selection := range selections {
		if selection.Identity.Kind == storage.SettingsCheckpointItemTarget {
			return true
		}
	}

	return false
}

func settingsRestoreRepositoryIDs(
	selections []storage.SettingsCheckpointRestoreSelection,
) []string {
	seen := map[string]bool{}
	for _, selection := range selections {
		if selection.Identity.RepositoryID != "" {
			seen[selection.Identity.RepositoryID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	return ids
}

func (s *Server) settingsCheckpointID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	checkpointID, err := strconv.ParseInt(r.PathValue("checkpoint"), 10, 64)
	if err != nil || checkpointID <= 0 {
		s.writeError(w, http.StatusNotFound, "not_found", "settings checkpoint not found")
		return 0, false
	}

	return checkpointID, true
}

func (s *Server) writeSettingsHistoryError(
	w http.ResponseWriter,
	err error,
	fallback func(http.ResponseWriter, error),
) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "not_found", "settings checkpoint not found")
	case errors.Is(err, storage.ErrConflict):
		s.writeError(w, http.StatusConflict, "conflict",
			"settings changed in another session; inspect the checkpoint again")
	case errors.Is(err, storage.ErrSettingsRestoreBlocked):
		s.writeError(w, http.StatusConflict, "settings_restore_blocked",
			"the selected settings cannot be restored")
	case errors.Is(err, storage.ErrSettingsRestoreNoop):
		s.writeError(w, http.StatusConflict, "settings_restore_noop",
			"the selected settings already match the checkpoint")
	case errors.Is(err, storage.ErrSettingsCheckpointCorrupt):
		s.writeError(w, http.StatusInternalServerError, "settings_checkpoint_corrupt",
			"the settings checkpoint failed its integrity check")
	default:
		fallback(w, err)
	}
}

func settingsCheckpointDTO(
	inspection storage.SettingsCheckpointInspection,
	actor storage.Account,
) settingsCheckpointResponse {
	checkpoint := inspection.Checkpoint
	answer := settingsCheckpointResponse{
		ID: strconv.FormatInt(checkpoint.ID, 10), Action: settingsCheckpointAction(checkpoint),
		Actor: accountDTO(actor), RestoredFromID: stringID(checkpoint.RestoredFromID),
		RestoredSide: checkpoint.RestoredSide,
		CreatedAt:    checkpoint.CreatedAt, AffectedKinds: []storage.SettingsCheckpointItemKind{},
		Items: make([]settingsCheckpointItemResponse, 0, len(inspection.Items)),
	}
	affected := map[storage.SettingsCheckpointItemKind]bool{}
	for _, item := range inspection.Items {
		if item.Changed {
			affected[item.Identity.Kind] = true
		}
		answer.Items = append(answer.Items, settingsCheckpointItemDTO(item))
	}
	for kind := range affected {
		answer.AffectedKinds = append(answer.AffectedKinds, kind)
	}
	sort.Slice(answer.AffectedKinds, func(left, right int) bool {
		return answer.AffectedKinds[left] < answer.AffectedKinds[right]
	})

	return answer
}

func settingsCheckpointItemDTO(
	item storage.SettingsCheckpointInspectionItem,
) settingsCheckpointItemResponse {
	return settingsCheckpointItemResponse{
		Kind: item.Identity.Kind, RepositoryID: item.Identity.RepositoryID,
		RepositoryFullName: item.RepositoryFullName, SyncKind: item.Identity.SyncKind,
		DocumentVersion: item.DocumentVersion,
		Before:          settingsCheckpointSideDTO(item.Before),
		After:           settingsCheckpointSideDTO(item.After),
		Current:         settingsCheckpointStateDTO(item.Current),
		Changed:         item.Changed,
	}
}

func settingsCheckpointSideDTO(
	side storage.SettingsCheckpointInspectionSide,
) settingsCheckpointSideResponse {
	return settingsCheckpointSideResponse{
		Available:       side.Available,
		State:           settingsCheckpointStateDTO(side.State),
		Differs:         side.Differs,
		Restorable:      side.Restorable,
		Incompatibility: settingsCheckpointIncompatibilityDTO(side.Incompatibility),
	}
}

func settingsCheckpointIncompatibilityDTO(
	value *storage.SettingsCheckpointIncompatibility,
) *settingsCheckpointIncompatibility {
	if value == nil {
		return nil
	}

	return &settingsCheckpointIncompatibility{Code: value.Code, Reason: value.Reason}
}

func settingsCheckpointStateDTO(
	state *storage.SettingsCheckpointState,
) *settingsCheckpointState {
	if state == nil {
		return nil
	}

	return &settingsCheckpointState{
		Document: append(json.RawMessage(nil), state.Document...),
		Digest:   state.Digest, Revision: state.Revision,
	}
}

func settingsCheckpointAction(checkpoint storage.SettingsCheckpoint) string {
	// The stored audit key, which is data rather than vocabulary: rows already
	// written say `installation.settings.*`, and renaming it here would orphan
	// every one of them.
	prefix := "installation"
	if checkpoint.Scope == storage.SettingsCheckpointScopeRoot {
		prefix = "runtime"
	}
	switch checkpoint.Action {
	case storage.SettingsCheckpointActionSave:
		return prefix + ".settings.saved"
	case storage.SettingsCheckpointActionRestore:
		return prefix + ".settings.restored"
	default:
		return prefix + ".settings.baseline"
	}
}

func workspaceSettingsRestoreAnswer(
	targetID string,
	result storage.SaveInstallationSettingsResult,
) workspaceSettingsBatchResponse {
	answer := workspaceSettingsBatchResponse{CheckpointID: stringID(result.CheckpointID)}
	if result.Target != nil {
		state := workspaceTargetSettingsStateFrom(*result.Target)
		answer.Target = &state
	}
	for _, repository := range result.Repositories {
		answer.Repositories = append(
			answer.Repositories, workspaceRepositorySettingsStateFrom(repository),
		)
	}
	sort.Slice(answer.Repositories, func(left, right int) bool {
		return answer.Repositories[left].RepositoryID < answer.Repositories[right].RepositoryID
	})
	answer.SyncConfigs = workspaceSyncConfigSettingsStates(targetID, result.SyncConfigs)
	answer.SyncOverrides = workspaceSyncOverrideSettingsStates(targetID, result.SyncOverrides)

	return answer
}
