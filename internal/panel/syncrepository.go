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
	if errors.Is(err, storage.ErrNotFound) {
		writeJSON(w, http.StatusOK, syncOverrideToDTO(kind, nil))

		return
	}

	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, syncOverrideToDTO(kind, &override))
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
	writeJSON(w, http.StatusOK, syncOverrideToDTO(kind, &saved))
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

	config, err := s.syncFileConfig(r, targetID)

	// An adjustment is checked against the files the installation
	// synchronizes, so one cannot be saved while those cannot be read. What a
	// repository wants left alone can: it names paths rather than fitting them,
	// and taking the one control that narrows sync away because of a problem
	// on somebody else's page would be taking it away at the worst moment.
	if err != nil && len(adjustments.Merges) > 0 {
		return nil, err
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
