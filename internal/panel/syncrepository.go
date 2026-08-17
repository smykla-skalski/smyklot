package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// syncOverrideRequest is what one repository says about one kind of sync.
//
// Both halves in one request, because they are one row and one revision.
// Sending them separately would let saving the switch write the last document
// this browser happened to be holding over one somebody else had just saved.
type syncOverrideRequest struct {
	// Enabled is a three-state answer: on, off, or absent, which inherits the
	// installation's. Present tells the third from a browser that forgot to
	// send the field.
	Enabled nullableBool `json:"enabled"`

	// Document is what this repository adjusts, as the kind spells it. Empty
	// adjusts nothing.
	Document json.RawMessage `json:"document,omitempty"`

	ExpectedRevision *int64 `json:"expected_revision"`
}

// syncOverrideDTO is what the panel reads back.
type syncOverrideDTO struct {
	Kind       string          `json:"kind"`
	Enabled    *bool           `json:"enabled"`
	Document   json.RawMessage `json:"document"`
	Revision   int64           `json:"revision"`
	UpdatedBy  string          `json:"updated_by,omitempty"`
	UpdatedAt  *time.Time      `json:"updated_at,omitempty"`
	Unreadable bool            `json:"unreadable"`

	// Problem is why this kind is not being synced here, and ProblemAt is when
	// the planner last found that. Absent where nothing is wrong.
	//
	// Read beside the override rather than left to the log, because this pane
	// is where somebody comes to ask why their repository is not getting the
	// organization's files - and, for two of the three reasons, it holds the
	// control that fixes it.
	Problem   string     `json:"problem,omitempty"`
	ProblemAt *time.Time `json:"problem_at,omitempty"`
}

// getSyncOverride reads what one repository says about one kind.
func (s *Server) getSyncOverride(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	repository, ok := s.repository(w, r, target)
	if !ok {
		return
	}
	kind, ok := s.syncKind(w, r)
	if !ok {
		return
	}

	override, err := s.store.GetSyncRepositoryOverride(
		r.Context(), target.ID, repository.ID, kind)

	// A repository that has said nothing about a kind inherits, which is an
	// answer this renders rather than a failure it reports.
	saved := &override
	if errors.Is(err, storage.ErrNotFound) {
		saved = nil
	} else if err != nil {
		s.writeStorageError(w, err)

		return
	}

	s.writeSyncOverride(w, r, repository.ID, kind, saved)
}

// writeSyncOverride answers with what this repository says about one kind and
// what the planner last made of it.
//
// Two rows and one question. The override is what somebody asked for; the state
// row is what came of it, and a pane showing only the first would show a
// repository that looks configured and is being skipped.
func (s *Server) writeSyncOverride(
	w http.ResponseWriter,
	r *http.Request,
	repositoryID string,
	kind orgsync.Kind,
	override *orgsync.RepositoryOverride,
) {
	dto := syncOverrideToDTO(kind, override)

	state, err := s.store.GetSyncRepositoryState(r.Context(), repositoryID, kind)

	switch {
	case errors.Is(err, storage.ErrNotFound):
		// Nothing has planned this repository for this kind yet, which is the
		// ordinary answer on a fresh installation and says nothing is wrong.
	case err != nil:
		// Reported rather than left out. A refusal this page could not read is
		// the one thing it exists to show, and rendering the pane without it
		// would say the repository is fine.
		s.writeStorageError(w, err)

		return
	default:
		dto.Problem = state.Problem
		if state.Problem != "" {
			dto.ProblemAt = &state.AppliedAt
		}
	}

	writeJSON(w, http.StatusOK, dto)
}

// putSyncOverride saves it.
//
// Validated against the installation's own configuration for the kind, because
// that is what the adjustment has to fit: one naming a file nobody synchronizes
// reads as configured and quietly does nothing, which is the same silence every
// other mistyped name here is refused for.
func (s *Server) putSyncOverride(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}
	repository, ok := s.repository(w, r, target)
	if !ok {
		return
	}
	kind, ok := s.syncKind(w, r)
	if !ok {
		return
	}

	var input syncOverrideRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !input.Enabled.Present || input.ExpectedRevision == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request",
			"a repository's answer needs to say whether the kind runs and what it replaces")

		return
	}

	document, err := s.syncOverrideDocument(r, target.ID, kind, input.Document)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_sync_override", err.Error())

		return
	}

	saved, err := s.store.SetSyncRepositoryOverride(r.Context(), orgsync.RepositoryOverrideChange{
		RepositoryID: repository.ID,
		Kind:         kind,
		Enabled:      input.Enabled.Value,
		Document:     document,
		ActorID:      account.ID,
		Now:          s.now().UTC(),
		Revision:     *input.ExpectedRevision,
	})
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	s.Announce(target.ID, repository.ID)

	// Carrying whatever refusal still stands, which a save does not clear. The
	// planner is what decides that, and it has not looked yet - saying so with
	// the time it was last looked at is honest, where dropping the notice would
	// tell somebody their fix worked before anything had tried it.
	s.writeSyncOverride(w, r, repository.ID, kind, &saved)
}

// syncOverrideDocument checks a repository's adjustments against what the
// installation synchronizes, and answers what to store.
func (s *Server) syncOverrideDocument(
	r *http.Request,
	targetID string,
	kind orgsync.Kind,
	document json.RawMessage,
) ([]byte, error) {
	if kind != orgsync.KindFiles {
		// Every other kind is the same everywhere the installation switches it
		// on, so a repository's answer about one is the switch and nothing else.
		// Refusing a document for them beats storing one nothing reads.
		if len(document) > 0 && string(document) != string(emptyDocument) {
			return nil, fmt.Errorf(
				"%w: a repository can switch %s off, and there is nothing about them to adjust",
				orgsync.ErrInvalidConfig, kind)
		}

		return nil, nil
	}

	var adjustments orgsync.FileOverride
	if err := decodeStrictly(document, &adjustments); err != nil {
		return nil, err
	}

	// An adjustment is checked against the files the installation
	// synchronizes, so those are read where there is an adjustment to check
	// and not otherwise. What a repository wants left alone names paths rather
	// than fitting them, so it saves whether or not that page can be read -
	// taking the one control that narrows sync away because of a problem on
	// somebody else's would take it away at the worst moment.
	var config orgsync.FileConfig

	if len(adjustments.Merges) > 0 {
		read, err := s.syncFileConfig(r, targetID)
		if err != nil {
			return nil, err
		}

		config = read
	}

	if err := adjustments.Validate(config); err != nil {
		return nil, err
	}

	return json.Marshal(adjustments)
}

// syncFileConfig reads what the installation synchronizes.
func (s *Server) syncFileConfig(r *http.Request, targetID string) (orgsync.FileConfig, error) {
	stored, err := s.store.GetSyncConfig(r.Context(), targetID, orgsync.KindFiles)
	if errors.Is(err, storage.ErrNotFound) {
		// Nothing configured, which is not an error: an adjustment naming a
		// file is then refused for naming one nobody synchronizes, which is the
		// truth.
		return orgsync.FileConfig{}, nil
	}
	if err != nil {
		return orgsync.FileConfig{}, err
	}

	var config orgsync.FileConfig
	if err := json.Unmarshal(stored.Document, &config); err != nil {
		return orgsync.FileConfig{}, fmt.Errorf(
			"%w: the files this installation synchronizes are stored in a form this "+
				"version cannot read, so an adjustment cannot be checked against them",
			orgsync.ErrInvalidConfig)
	}

	return config, nil
}

// syncOverrideToDTO renders a repository's answer, including the one it has
// never given.
func syncOverrideToDTO(kind orgsync.Kind, override *orgsync.RepositoryOverride) syncOverrideDTO {
	if override == nil {
		// Never answered, which is not the same as answered and switched off.
		// One shape either way, so a browser has one thing to read.
		return syncOverrideDTO{Kind: string(kind), Document: emptyDocument}
	}

	dto := syncOverrideDTO{
		Kind:      string(kind),
		Enabled:   override.Enabled,
		Document:  documentOrEmpty(override.Document),
		Revision:  override.Revision,
		UpdatedBy: override.UpdatedBy,
		UpdatedAt: &override.UpdatedAt,
	}

	// Bytes that are not JSON, before they are carried anywhere. A
	// json.RawMessage is validated as it is copied out, so holding one that
	// does not parse fails the whole response rather than this field.
	if !json.Valid(dto.Document) {
		dto.Document = emptyDocument
		dto.Unreadable = true
	}

	return dto
}
