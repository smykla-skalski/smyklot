package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

const maxSyncConfigBatchBody = maxDocumentBody + 3*maxRequestBody

type syncConfigBatchRequest struct {
	Changes []syncConfigBatchItemRequest `json:"changes"`
}

type syncConfigBatchItemRequest struct {
	Kind             string          `json:"kind"`
	Enabled          *bool           `json:"enabled"`
	Labels           []orgsync.Label `json:"labels"`
	AllowRemoval     bool            `json:"allow_removal"`
	Excludes         []string        `json:"excludes"`
	ExpectedRevision *int64          `json:"expected_revision"`
	Document         json.RawMessage `json:"document,omitempty"`
}

type syncConfigBatchResponse struct {
	Configs      []syncConfigDTO `json:"configs"`
	CheckpointID *string         `json:"checkpoint_id,omitempty"`
}

type syncConfigErrorResponse struct {
	Error syncConfigError `json:"error"`
}

type syncConfigError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Kind    string `json:"kind"`
}

type syncConfigRestoreRequest struct {
	Kinds []syncConfigRestoreKindRequest `json:"kinds"`
}

type syncConfigRestoreKindRequest struct {
	Kind             string `json:"kind"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

type syncConfigCheckpointResponse struct {
	ID             string                             `json:"id"`
	Action         string                             `json:"action"`
	Actor          accountResponse                    `json:"actor"`
	RestoredFromID *string                            `json:"restored_from_id,omitempty"`
	CreatedAt      time.Time                          `json:"created_at"`
	AffectedKinds  []string                           `json:"affected_kinds"`
	Kinds          []syncConfigCheckpointKindResponse `json:"kinds"`
}

type syncConfigCheckpointKindResponse struct {
	Kind               string                     `json:"kind"`
	Before             *syncConfigCheckpointState `json:"before"`
	After              *syncConfigCheckpointState `json:"after"`
	Current            *syncConfigCheckpointState `json:"current"`
	Changed            bool                       `json:"changed"`
	DiffersFromCurrent bool                       `json:"differs_from_current"`
}

type syncConfigCheckpointState struct {
	Enabled  bool            `json:"enabled"`
	Document json.RawMessage `json:"document"`
	Digest   string          `json:"digest"`
	Revision int64           `json:"revision"`
}

func (s *Server) putSyncConfigs(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}

	var input syncConfigBatchRequest
	if !decodeJSONWithin(w, r, &input, maxSyncConfigBatchBody) {
		return
	}
	patches, ok := s.syncConfigPatches(w, input)
	if !ok {
		return
	}

	written, err := s.store.SetSyncConfigs(r.Context(), orgsync.ConfigBatchChange{
		TargetID: target.ID, ActorID: account.ID, Now: s.now().UTC(), Changes: patches,
	})
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	answer, err := s.syncConfigBatchAnswer(r, target, written)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if written.CheckpointID != nil {
		s.Announce(target.ID, "")
	}
	writeJSON(w, http.StatusOK, answer)
}

func (s *Server) syncConfigPatches(
	w http.ResponseWriter,
	input syncConfigBatchRequest,
) ([]orgsync.ConfigPatch, bool) {
	if len(input.Changes) == 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_request",
			"a sync configuration save needs at least one changed kind")
		return nil, false
	}
	seen := make(map[orgsync.Kind]bool, len(input.Changes))
	patches := make([]orgsync.ConfigPatch, 0, len(input.Changes))
	for _, change := range input.Changes {
		kind := orgsync.Kind(change.Kind)
		if !kind.Valid() || seen[kind] {
			s.writeError(w, http.StatusBadRequest, "invalid_request",
				"each changed sync kind must be known and appear once")
			return nil, false
		}
		seen[kind] = true
		if change.Enabled == nil || change.ExpectedRevision == nil || *change.ExpectedRevision < 0 {
			s.writeError(w, http.StatusBadRequest, "invalid_request",
				"each changed sync kind needs enabled and a current expected revision")
			return nil, false
		}
		encoded, err := json.Marshal(change)
		if err != nil {
			s.writeInternal(w, err)
			return nil, false
		}
		if int64(len(encoded)) > bodyBoundFor(kind) {
			s.writeError(w, http.StatusRequestEntityTooLarge, "request_too_large",
				fmt.Sprintf("the %s sync configuration is larger than %d bytes",
					kind, bodyBoundFor(kind)))
			return nil, false
		}
		document, err := syncDocumentFor(kind, syncConfigRequest{
			Labels: change.Labels, AllowRemoval: change.AllowRemoval, Excludes: change.Excludes,
			Document: change.Document,
		})
		if err != nil {
			writeSyncConfigError(w, kind, err)
			return nil, false
		}
		patches = append(patches, orgsync.ConfigPatch{
			Kind: kind, Enabled: *change.Enabled, Document: document,
			Revision: *change.ExpectedRevision,
		})
	}

	return patches, true
}

func writeSyncConfigError(w http.ResponseWriter, kind orgsync.Kind, err error) {
	writeJSON(w, http.StatusBadRequest, syncConfigErrorResponse{Error: syncConfigError{
		Code: "invalid_sync_config", Message: err.Error(), Kind: string(kind),
	}})
}

func (s *Server) syncConfigBatchAnswer(
	r *http.Request,
	target storage.Target,
	written orgsync.ConfigWrite,
) (syncConfigBatchResponse, error) {
	configs := make(map[orgsync.Kind]orgsync.Config, len(written.Configs))
	for _, config := range written.Configs {
		configs[config.Kind] = config
	}
	answer := syncConfigBatchResponse{Configs: make([]syncConfigDTO, 0, len(orgsync.Kinds()))}
	for _, kind := range orgsync.Kinds() {
		config, found := configs[kind]
		if !found {
			config = orgsync.Config{Kind: kind}
		}
		login, err := s.syncEditorLogin(r.Context(), config.UpdatedBy)
		if err != nil {
			return syncConfigBatchResponse{}, err
		}
		answer.Configs = append(answer.Configs, syncConfigAnswer(config, target, login))
	}
	answer.CheckpointID = stringID(written.CheckpointID)

	return answer, nil
}

func (s *Server) getSyncConfigCheckpoint(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	s.writeSyncConfigCheckpoint(w, r, target)
}

func (s *Server) getRootSyncConfigCheckpoint(w http.ResponseWriter, r *http.Request) {
	context, ok := s.requireRootTarget(w, r, false)
	if !ok {
		return
	}
	s.writeSyncConfigCheckpoint(w, r, context.Target)
}

func (s *Server) writeSyncConfigCheckpoint(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
) {
	checkpointID, ok := s.syncCheckpointID(w, r)
	if !ok {
		return
	}

	checkpoint, err := s.store.GetSyncConfigCheckpoint(r.Context(), target.ID, checkpointID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	current, err := s.store.ListSyncConfigs(r.Context(), target.ID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	actor, err := s.store.GetAccount(r.Context(), checkpoint.ActorID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, syncConfigCheckpointDTO(checkpoint, current, actor))
}

func (s *Server) postSyncConfigRestore(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}
	s.restoreSyncConfigCheckpoint(w, r, target, syncRestoreActor{
		AccountID: account.ID, WriteError: s.writeStorageError,
	})
}

func (s *Server) postRootSyncConfigRestore(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	context, ok := s.requireRootTarget(w, r, true)
	if !ok {
		return
	}
	s.restoreSyncConfigCheckpoint(w, r, context.Target, syncRestoreActor{
		AccountID: context.Account.ID, ElevationID: elevationID(context.Elevation),
		SessionTokenHash: context.SessionHash, WriteError: s.writeRootWriteError,
	})
}

type syncRestoreActor struct {
	AccountID        string
	ElevationID      *string
	SessionTokenHash string
	WriteError       func(http.ResponseWriter, error)
}

func (s *Server) restoreSyncConfigCheckpoint(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
	actor syncRestoreActor,
) {
	checkpointID, ok := s.syncCheckpointID(w, r)
	if !ok {
		return
	}

	var input syncConfigRestoreRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	kinds, revisions, ok := s.syncRestoreSelection(w, input)
	if !ok {
		return
	}
	checkpoint, err := s.store.GetSyncConfigCheckpoint(r.Context(), target.ID, checkpointID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if !validateSyncRestore(w, checkpoint, kinds) {
		return
	}

	written, err := s.store.RestoreSyncConfigCheckpoint(r.Context(), orgsync.ConfigRestore{
		TargetID: target.ID, CheckpointID: checkpointID, Kinds: kinds,
		Revisions: revisions, ActorID: actor.AccountID, ElevationID: actor.ElevationID,
		SessionTokenHash: actor.SessionTokenHash, Now: s.now().UTC(),
	})
	if err != nil {
		actor.WriteError(w, err)
		return
	}
	answer, err := s.syncConfigBatchAnswer(r, target, written)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}
	if written.CheckpointID != nil {
		s.Announce(target.ID, "")
	}
	writeJSON(w, http.StatusOK, answer)
}

func (s *Server) syncRestoreSelection(
	w http.ResponseWriter,
	input syncConfigRestoreRequest,
) ([]orgsync.Kind, map[orgsync.Kind]int64, bool) {
	if len(input.Kinds) == 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_request",
			"restore needs at least one selected sync kind")
		return nil, nil, false
	}
	kinds := make([]orgsync.Kind, 0, len(input.Kinds))
	revisions := make(map[orgsync.Kind]int64, len(input.Kinds))
	for _, selection := range input.Kinds {
		kind := orgsync.Kind(selection.Kind)
		_, duplicate := revisions[kind]
		if !kind.Valid() || duplicate || selection.ExpectedRevision == nil ||
			*selection.ExpectedRevision < 0 {
			s.writeError(w, http.StatusBadRequest, "invalid_request",
				"each restored sync kind must be known, unique, and name its current revision")
			return nil, nil, false
		}
		kinds = append(kinds, kind)
		revisions[kind] = *selection.ExpectedRevision
	}

	return kinds, revisions, true
}

func validateSyncRestore(
	w http.ResponseWriter,
	checkpoint orgsync.ConfigCheckpoint,
	kinds []orgsync.Kind,
) bool {
	items := checkpointItemMap(checkpoint.Items)
	for _, kind := range kinds {
		item, present := items[kind]
		if !present {
			continue
		}
		if orgsync.DigestConfig(item.Enabled, item.Document) != item.Digest {
			writeSyncConfigError(w, kind, errors.New("the historical state failed its integrity check"))
			return false
		}
		if err := validateHistoricalSyncDocument(kind, item.Document); err != nil {
			writeSyncConfigError(w, kind,
				fmt.Errorf("this historical state is not compatible with the current version: %w", err))
			return false
		}
	}

	return true
}

func validateHistoricalSyncDocument(kind orgsync.Kind, document []byte) error {
	switch kind {
	case orgsync.KindLabels:
		return validateStoredDocument[orgsync.LabelConfig](document)
	case orgsync.KindSettings:
		return validateStoredDocument[orgsync.SettingsConfig](document)
	case orgsync.KindRulesets:
		return validateStoredDocument[orgsync.RulesetConfig](document)
	case orgsync.KindFiles:
		return validateStoredDocument[orgsync.FileConfig](document)
	default:
		return fmt.Errorf("%w: unknown kind %q", orgsync.ErrInvalidConfig, kind)
	}
}

func validateStoredDocument[T interface{ Validate() error }](document []byte) error {
	var config T
	if err := decodeStrictly(document, &config); err != nil {
		return err
	}
	return config.Validate()
}

func (s *Server) syncCheckpointID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	checkpointID, err := strconv.ParseInt(r.PathValue("checkpoint"), 10, 64)
	if err != nil || checkpointID <= 0 {
		s.writeError(w, http.StatusNotFound, "not_found", "sync checkpoint not found")
		return 0, false
	}

	return checkpointID, true
}

func syncConfigCheckpointDTO(
	checkpoint orgsync.ConfigCheckpoint,
	current []orgsync.Config,
	actor storage.Account,
) syncConfigCheckpointResponse {
	before := checkpointItemMap(checkpoint.PreviousItems)
	after := checkpointItemMap(checkpoint.Items)
	currentItems := currentCheckpointItemMap(current)
	answer := syncConfigCheckpointResponse{
		ID: strconv.FormatInt(checkpoint.ID, 10), Action: checkpointAuditAction(checkpoint.Action),
		Actor: accountDTO(actor), RestoredFromID: stringID(checkpoint.RestoredFromID),
		CreatedAt: checkpoint.CreatedAt, AffectedKinds: []string{},
		Kinds: make([]syncConfigCheckpointKindResponse, 0, len(orgsync.Kinds())),
	}
	for _, kind := range orgsync.Kinds() {
		beforeItem, beforePresent := before[kind]
		afterItem, afterPresent := after[kind]
		currentItem, currentPresent := currentItems[kind]
		changed := !checkpointStatesEqual(beforeItem, beforePresent, afterItem, afterPresent)
		if changed {
			answer.AffectedKinds = append(answer.AffectedKinds, string(kind))
		}
		answer.Kinds = append(answer.Kinds, syncConfigCheckpointKindResponse{
			Kind: string(kind), Before: checkpointState(beforeItem, beforePresent),
			After:   checkpointState(afterItem, afterPresent),
			Current: checkpointState(currentItem, currentPresent), Changed: changed,
			DiffersFromCurrent: !checkpointStatesEqual(afterItem, afterPresent, currentItem, currentPresent),
		})
	}

	return answer
}

func checkpointItemMap(
	items []orgsync.ConfigCheckpointItem,
) map[orgsync.Kind]orgsync.ConfigCheckpointItem {
	result := make(map[orgsync.Kind]orgsync.ConfigCheckpointItem, len(items))
	for _, item := range items {
		result[item.Kind] = item
	}
	return result
}

func currentCheckpointItemMap(
	configs []orgsync.Config,
) map[orgsync.Kind]orgsync.ConfigCheckpointItem {
	items := make(map[orgsync.Kind]orgsync.ConfigCheckpointItem, len(configs))
	for _, config := range configs {
		items[config.Kind] = orgsync.ConfigCheckpointItem{
			Kind: config.Kind, Enabled: config.Enabled, Document: config.Document,
			Digest: config.Digest, Revision: config.Revision,
		}
	}
	return items
}

func checkpointStatesEqual(
	left orgsync.ConfigCheckpointItem,
	leftPresent bool,
	right orgsync.ConfigCheckpointItem,
	rightPresent bool,
) bool {
	return leftPresent == rightPresent && (!leftPresent || left.Digest == right.Digest)
}

func checkpointState(
	item orgsync.ConfigCheckpointItem,
	present bool,
) *syncConfigCheckpointState {
	if !present {
		return nil
	}
	return &syncConfigCheckpointState{
		Enabled: item.Enabled, Document: documentOrEmpty(item.Document),
		Digest: item.Digest, Revision: item.Revision,
	}
}

func checkpointAuditAction(action orgsync.CheckpointAction) string {
	switch action {
	case orgsync.CheckpointSaved:
		return "sync.config.saved"
	case orgsync.CheckpointRestored:
		return "sync.config.restored"
	default:
		return "sync.config.baseline"
	}
}

func stringID(id *int64) *string {
	if id == nil {
		return nil
	}
	formatted := strconv.FormatInt(*id, 10)
	return &formatted
}
