package panel

import (
	"encoding/json"
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

type rootRuntimeRestoreRequest struct {
	State      string                        `json:"state"`
	Selections []rootRuntimeRestoreSelection `json:"selections"`
}

type rootRuntimeRestoreSelection struct {
	Kind             string `json:"kind"`
	ExpectedRevision *int64 `json:"expected_revision"`
}

func (s *Server) getRootSettingsCheckpoint(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	checkpointID, ok := s.settingsCheckpointID(w, r)
	if !ok {
		return
	}
	inspection, err := s.store.InspectRootSettingsCheckpoint(
		r.Context(),
		storage.SettingsCheckpointRef{
			ID:    checkpointID,
			Scope: storage.SettingsCheckpointScopeRoot,
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

	writeJSON(w, http.StatusOK, settingsCheckpointDTO(inspection, actor))
}

func (s *Server) getRootSettingsBaseline(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	inspection, err := s.store.InspectRootSettingsBaseline(r.Context())
	if err != nil {
		s.writeSettingsHistoryError(w, err, s.writeStorageError)
		return
	}
	actor, err := s.store.GetAccount(r.Context(), inspection.Checkpoint.ActorAccountID)
	if err != nil {
		s.writeStorageError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, settingsCheckpointDTO(inspection, actor))
}

func (s *Server) postRootSettingsRestore(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	actor, _, ok := s.requireRoot(w, r)
	if !ok {
		return
	}
	checkpointID, ok := s.settingsCheckpointID(w, r)
	if !ok {
		return
	}
	selection, side, ok := s.rootRuntimeRestoreSelection(w, r)
	if !ok {
		return
	}
	inspection, err := s.store.InspectRootSettingsCheckpoint(
		r.Context(),
		storage.SettingsCheckpointRef{
			ID:    checkpointID,
			Scope: storage.SettingsCheckpointScopeRoot,
		},
	)
	if err != nil {
		s.writeSettingsHistoryError(w, err, s.writeStorageError)
		return
	}
	document, ok := rootRuntimeCheckpointDocument(inspection, side)
	if !ok {
		s.writeSettingsHistoryError(w, storage.ErrSettingsRestoreBlocked, s.writeStorageError)
		return
	}
	proposed := storage.RuntimeSettings{
		BotConfig:            document.BotConfig,
		LogLevel:             document.LogLevel,
		PollInterval:         document.PollInterval,
		PendingCIQuietPeriod: document.PendingCIQuietPeriod,
		SessionTTL:           document.SessionTTL,
		PathIndexInterval:    document.PathIndexInterval,
	}
	effective, err := resolveRuntimeValues(s.cfg, proposed)
	if err != nil {
		s.writeSettingsHistoryError(w, storage.ErrSettingsRestoreBlocked, s.writeStorageError)
		return
	}
	result, err := s.store.RestoreRuntimeSettings(
		r.Context(),
		storage.RestoreRuntimeSettingsRequest{
			CheckpointID:                  checkpointID,
			Side:                          side,
			ExpectedRevision:              *selection.ExpectedRevision,
			ActorAccountID:                actor.ID,
			ChangedAt:                     s.now().UTC(),
			Runner:                        s.cfg.ProcessConfig.EffectiveRunner(),
			EffectivePendingCIQuietPeriod: effective.PendingCIQuietPeriod,
			EffectivePollInterval:         effective.PollInterval,
			EffectivePathIndexInterval:    effective.PathIndexInterval,
			EffectiveSessionTTL:           effective.SessionTTL,
		},
	)
	if err != nil {
		s.writeSettingsHistoryError(w, err, s.writeStorageError)
		return
	}
	if err := s.applyRuntimeSettings(result.Settings); err != nil {
		s.writeInternal(w, err)
		return
	}
	s.events.announce(panelEvent{Type: panelEventResync})
	writeJSON(w, http.StatusOK, runtimeSettingsSaveDTO(
		result,
		s.store.Status(r.Context()),
		s.cfg,
		s.runtimeValues(),
		s.startedAt,
		s.now().UTC(),
	))
}

func (s *Server) rootRuntimeRestoreSelection(
	w http.ResponseWriter,
	r *http.Request,
) (rootRuntimeRestoreSelection, storage.SettingsCheckpointRestoreSide, bool) {
	var input rootRuntimeRestoreRequest
	if !decodeJSONWithin(w, r, &input, maxWorkspaceSettingsRestoreBody) {
		return rootRuntimeRestoreSelection{}, "", false
	}
	side := storage.SettingsCheckpointRestoreSide(input.State)
	if len(input.Selections) != 1 || !side.Valid() {
		s.writeInvalidRootRuntimeRestore(w)
		return rootRuntimeRestoreSelection{}, "", false
	}
	selection := input.Selections[0]
	if selection.Kind != string(storage.SettingsCheckpointItemRuntime) ||
		selection.ExpectedRevision == nil || *selection.ExpectedRevision < 0 {
		s.writeInvalidRootRuntimeRestore(w)
		return rootRuntimeRestoreSelection{}, "", false
	}

	return selection, side, true
}

func (s *Server) writeInvalidRootRuntimeRestore(w http.ResponseWriter) {
	s.writeError(
		w,
		http.StatusBadRequest,
		"invalid_request",
		"Root restore must select runtime settings at their current revision",
	)
}

func rootRuntimeCheckpointDocument(
	inspection storage.SettingsCheckpointInspection,
	side storage.SettingsCheckpointRestoreSide,
) (storage.RuntimeSettingsDocument, bool) {
	for _, item := range inspection.Items {
		selected := selectedSettingsCheckpointSide(item, side)
		if item.Identity.Kind != storage.SettingsCheckpointItemRuntime ||
			!selected.Restorable || selected.State == nil {
			continue
		}
		var document storage.RuntimeSettingsDocument
		if err := json.Unmarshal(selected.State.Document, &document); err != nil {
			return storage.RuntimeSettingsDocument{}, false
		}

		return document, true
	}

	return storage.RuntimeSettingsDocument{}, false
}

func selectedSettingsCheckpointSide(
	item storage.SettingsCheckpointInspectionItem,
	side storage.SettingsCheckpointRestoreSide,
) storage.SettingsCheckpointInspectionSide {
	if side == storage.SettingsCheckpointRestoreBefore {
		return item.Before
	}

	return item.After
}
