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

const maxInstallationSettingsRestoreBody = 1 << 20

type installationSettingsRestoreRequest struct {
	Selections []installationSettingsRestoreSelectionRequest `json:"selections"`
}

type installationSettingsRestoreSelectionRequest struct {
	Kind             string `json:"kind"`
	RepositoryID     string `json:"repository_id,omitempty"`
	SyncKind         string `json:"sync_kind,omitempty"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

type installationSettingsCheckpointResponse struct {
	ID             string                                       `json:"id"`
	Action         string                                       `json:"action"`
	Actor          accountResponse                              `json:"actor"`
	RestoredFromID *string                                      `json:"restored_from_id,omitempty"`
	CreatedAt      time.Time                                    `json:"created_at"`
	AffectedKinds  []storage.SettingsCheckpointItemKind         `json:"affected_kinds"`
	Items          []installationSettingsCheckpointItemResponse `json:"items"`
}

type installationSettingsCheckpointItemResponse struct {
	Kind               storage.SettingsCheckpointItemKind   `json:"kind"`
	RepositoryID       string                               `json:"repository_id,omitempty"`
	RepositoryFullName string                               `json:"repository_full_name,omitempty"`
	SyncKind           orgsync.Kind                         `json:"sync_kind,omitempty"`
	DocumentVersion    int                                  `json:"document_version"`
	Before             *installationSettingsCheckpointState `json:"before"`
	After              *installationSettingsCheckpointState `json:"after"`
	Current            *installationSettingsCheckpointState `json:"current"`
	Changed            bool                                 `json:"changed"`
	Differs            bool                                 `json:"differs"`
	Restorable         bool                                 `json:"restorable"`
	Incompatibility    *installationSettingsIncompatibility `json:"incompatibility,omitempty"`
}

type installationSettingsCheckpointState struct {
	Document json.RawMessage `json:"document"`
	Digest   string          `json:"digest"`
	Revision int64           `json:"revision"`
}

type installationSettingsIncompatibility struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type installationSettingsRestoreActor struct {
	accountID        string
	elevationID      *string
	sessionTokenHash string
	root             bool
	writeError       func(http.ResponseWriter, error)
}

func (s *Server) getInstallationSettingsCheckpoint(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	s.writeInstallationSettingsCheckpoint(w, r, target)
}

func (s *Server) getRootInstallationSettingsCheckpoint(w http.ResponseWriter, r *http.Request) {
	root, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	s.writeInstallationSettingsCheckpoint(w, r, root.Target)
}

func (s *Server) writeInstallationSettingsCheckpoint(
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
	actor, err := s.store.GetAccount(r.Context(), inspection.Checkpoint.ActorAccountID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, installationSettingsCheckpointDTO(inspection, actor))
}

func (s *Server) postInstallationSettingsRestore(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}
	s.restoreInstallationSettingsCheckpoint(w, r, target, installationSettingsRestoreActor{
		accountID: account.ID, writeError: s.writeStorageError,
	})
}

func (s *Server) postRootInstallationSettingsRestore(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	root, ok := s.requireRootTarget(w, r, true)
	if !ok {
		return
	}
	s.restoreInstallationSettingsCheckpoint(w, r, root.Target, installationSettingsRestoreActor{
		accountID: root.Account.ID, elevationID: elevationID(root.Elevation),
		sessionTokenHash: root.SessionHash, root: true, writeError: s.writeRootWriteError,
	})
}

func (s *Server) restoreInstallationSettingsCheckpoint(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
	actor installationSettingsRestoreActor,
) {
	request, ok := s.prepareInstallationSettingsRestore(w, r, target.ID, actor)
	if !ok {
		return
	}
	result, err := s.restoreInstallationSettings(r.Context(), request)
	if err != nil {
		s.writeInstallationSettingsRestoreError(w, err, actor)
		return
	}
	s.signalInstallationSettingsBatch(target.ID, result)
	writeJSON(w, http.StatusOK, installationSettingsRestoreAnswer(target.ID, result))
}

func (s *Server) writeInstallationSettingsRestoreError(
	w http.ResponseWriter,
	err error,
	actor installationSettingsRestoreActor,
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

func (s *Server) prepareInstallationSettingsRestore(
	w http.ResponseWriter,
	r *http.Request,
	targetID string,
	actor installationSettingsRestoreActor,
) (storage.RestoreInstallationSettingsRequest, bool) {
	checkpointID, ok := s.settingsCheckpointID(w, r)
	if !ok {
		return storage.RestoreInstallationSettingsRequest{}, false
	}
	var input installationSettingsRestoreRequest
	if !decodeJSONWithin(w, r, &input, maxInstallationSettingsRestoreBody) {
		return storage.RestoreInstallationSettingsRequest{}, false
	}
	request := storage.RestoreInstallationSettingsRequest{
		TargetID: targetID, CheckpointID: checkpointID, ActorAccountID: actor.accountID,
		ElevationID: actor.elevationID, SessionTokenHash: actor.sessionTokenHash,
		ChangedAt: s.now().UTC(), DeploymentPendingCIQuietPeriod: s.cfg.PendingCIQuietPeriod,
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

func (s *Server) restoreInstallationSettings(
	ctx context.Context,
	request storage.RestoreInstallationSettingsRequest,
) (storage.SaveInstallationSettingsResult, error) {
	operation := func() (storage.SaveInstallationSettingsResult, error) {
		return s.store.RestoreInstallationSettings(ctx, request)
	}
	if settingsRestoreIncludesTarget(request.Selections) {
		return s.saveInstallationTargetSettingsBatch(ctx, request.TargetID, operation)
	}
	repositoryIDs := settingsRestoreRepositoryIDs(request.Selections)
	if len(repositoryIDs) == 0 {
		return operation()
	}

	return saveInstallationSettingsExclusive(s, ctx, repositoryIDs, operation)
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

func installationSettingsCheckpointDTO(
	inspection storage.InstallationSettingsCheckpointInspection,
	actor storage.Account,
) installationSettingsCheckpointResponse {
	checkpoint := inspection.Checkpoint
	answer := installationSettingsCheckpointResponse{
		ID: strconv.FormatInt(checkpoint.ID, 10), Action: settingsCheckpointAction(checkpoint.Action),
		Actor: accountDTO(actor), RestoredFromID: stringID(checkpoint.RestoredFromID),
		CreatedAt: checkpoint.CreatedAt, AffectedKinds: []storage.SettingsCheckpointItemKind{},
		Items: make([]installationSettingsCheckpointItemResponse, 0, len(inspection.Items)),
	}
	affected := map[storage.SettingsCheckpointItemKind]bool{}
	for _, item := range inspection.Items {
		changed := settingsCheckpointStatesDiffer(item.Before, item.After)
		if changed {
			affected[item.Identity.Kind] = true
		}
		answer.Items = append(answer.Items, installationSettingsCheckpointItemDTO(item, changed))
	}
	for kind := range affected {
		answer.AffectedKinds = append(answer.AffectedKinds, kind)
	}
	sort.Slice(answer.AffectedKinds, func(left, right int) bool {
		return answer.AffectedKinds[left] < answer.AffectedKinds[right]
	})

	return answer
}

func installationSettingsCheckpointItemDTO(
	item storage.SettingsCheckpointInspectionItem,
	changed bool,
) installationSettingsCheckpointItemResponse {
	return installationSettingsCheckpointItemResponse{
		Kind: item.Identity.Kind, RepositoryID: item.Identity.RepositoryID,
		RepositoryFullName: item.RepositoryFullName, SyncKind: item.Identity.SyncKind,
		DocumentVersion: item.DocumentVersion,
		Before:          installationSettingsCheckpointStateDTO(item.Before),
		After:           installationSettingsCheckpointStateDTO(item.After),
		Current:         installationSettingsCheckpointStateDTO(item.Current),
		Changed:         changed, Differs: item.Differs, Restorable: item.Restorable,
		Incompatibility: installationSettingsIncompatibilityDTO(item.Incompatibility),
	}
}

func installationSettingsIncompatibilityDTO(
	value *storage.SettingsCheckpointIncompatibility,
) *installationSettingsIncompatibility {
	if value == nil {
		return nil
	}

	return &installationSettingsIncompatibility{Code: value.Code, Reason: value.Reason}
}

func installationSettingsCheckpointStateDTO(
	state *storage.SettingsCheckpointState,
) *installationSettingsCheckpointState {
	if state == nil {
		return nil
	}

	return &installationSettingsCheckpointState{
		Document: append(json.RawMessage(nil), state.Document...),
		Digest:   state.Digest, Revision: state.Revision,
	}
}

func settingsCheckpointStatesDiffer(
	before *storage.SettingsCheckpointState,
	after *storage.SettingsCheckpointState,
) bool {
	if before == nil || after == nil {
		return before != nil || after != nil
	}

	return before.Digest != after.Digest
}

func settingsCheckpointAction(action storage.SettingsCheckpointAction) string {
	switch action {
	case storage.SettingsCheckpointActionSave:
		return "installation.settings.saved"
	case storage.SettingsCheckpointActionRestore:
		return "installation.settings.restored"
	default:
		return "installation.settings.baseline"
	}
}

func installationSettingsRestoreAnswer(
	targetID string,
	result storage.SaveInstallationSettingsResult,
) installationSettingsBatchResponse {
	answer := installationSettingsBatchResponse{CheckpointID: stringID(result.CheckpointID)}
	if result.Target != nil {
		state := installationTargetSettingsStateFrom(*result.Target)
		answer.Target = &state
	}
	for _, repository := range result.Repositories {
		answer.Repositories = append(
			answer.Repositories, installationRepositorySettingsStateFrom(repository),
		)
	}
	sort.Slice(answer.Repositories, func(left, right int) bool {
		return answer.Repositories[left].RepositoryID < answer.Repositories[right].RepositoryID
	})
	answer.SyncConfigs = installationSyncConfigSettingsStates(targetID, result.SyncConfigs)
	answer.SyncOverrides = installationSyncOverrideSettingsStates(targetID, result.SyncOverrides)

	return answer
}
