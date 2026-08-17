package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
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

	// Reporting an unreadable state row, because on a read it is the one thing
	// this page exists to show.
	s.answerSyncOverride(w, r, target.ID, repository.ID, kind, saved, true)
}

// answerSyncOverride answers with what this repository says about one kind and
// what the planner last made of it.
//
// Two rows and one question. The override is what somebody asked for; the state
// row is what came of it, and a pane showing only the first would show a
// repository that looks configured and is being skipped.
//
// reportUnreadableState is what separates the two callers. On a read, a refusal
// this page could not read is the one thing it exists to show, so the request
// fails. After a save it is a second read on the way out of a write that has
// already committed, and failing there would answer 500 for a change that
// landed - which the form reads as a failed save, so it keeps the revision it
// came in with and every retry is answered 409 for the person's own change.
func (s *Server) answerSyncOverride(
	w http.ResponseWriter,
	r *http.Request,
	targetID, repositoryID string,
	kind orgsync.Kind,
	override *orgsync.RepositoryOverride,
	reportUnreadableState bool,
) {
	dto := syncOverrideToDTO(kind, override)

	if override.Disabled() {
		// Switched off here, so the planner is not looking - and a row is only
		// rewritten while it is. Whatever reason was recorded last is frozen at
		// the moment somebody turned the kind off, which is usually the moment
		// they turned it off *because* of, so it would be rendered as a live
		// notice directly under a control reading "Disabled".
		writeJSON(w, http.StatusOK, dto)

		return
	}

	state, err := s.store.GetSyncRepositoryState(r.Context(), targetID, repositoryID, kind)

	switch {
	case errors.Is(err, storage.ErrNotFound):
		// Nothing has planned this repository for this kind yet, which is the
		// ordinary answer on a fresh installation and says nothing is wrong.
	case err != nil && reportUnreadableState:
		// Reported rather than left out. A refusal this page could not read is
		// the one thing it exists to show, and rendering the pane without it
		// would say the repository is fine.
		s.writeStorageError(w, err)

		return
	case err != nil:
		// Left out, because the save this is answering has already landed.
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

	// The ordinary bound, because a repository's adjustments are not templates.
	// FileOverride bounds neither how many merges it carries nor how large an
	// overrides object is, so the larger bound would buy a per-repository row of
	// several megabytes that the planner then refuses to compose anyway.
	var input syncOverrideRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !input.Enabled.Present || input.ExpectedRevision == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request",
			"a repository's answer needs to say whether the kind runs and what it replaces")

		return
	}

	document, err := s.syncOverrideDocument(
		r, target.ID, repository.ID, kind, input.Document)
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
	// Not reporting an unreadable state row: the save has already committed.
	s.answerSyncOverride(w, r, target.ID, repository.ID, kind, &saved, false)
}

// syncOverrideDocument checks a repository's adjustments against what the
// installation synchronizes, and answers what to store.
func (s *Server) syncOverrideDocument(
	r *http.Request,
	targetID, repositoryID string,
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

	keeping := s.alreadyAdjusted(r, targetID, repositoryID)

	// An adjustment is checked against the files the installation
	// synchronizes, so those are read where this save introduces one to check
	// and not otherwise.
	//
	// Two things are not introduced. What a repository wants left alone names
	// paths rather than fitting them, and an adjustment it already had was
	// checked when it was saved. Neither should turn on a page that may be
	// unreadable for reasons on somebody else's screen: taking the one control
	// that narrows sync away because of a problem elsewhere would take it away
	// at the worst moment, and the form always re-sends the whole document, so
	// every save carried every stored adjustment back through this.
	var config orgsync.FileConfig

	if adjustsBeyond(adjustments, keeping) {
		read, err := s.syncFileConfig(r, targetID)
		if err != nil {
			return nil, err
		}

		config = read
	}

	if err := adjustments.ValidateAgainst(config, keeping); err != nil {
		return nil, err
	}

	return json.Marshal(adjustments)
}

// adjustsBeyond reports a save naming a path this repository was not already
// adjusting, which is the only thing the installation's configuration decides.
func adjustsBeyond(adjustments orgsync.FileOverride, keeping []string) bool {
	for _, path := range adjustments.Adjusted() {
		if !slices.Contains(keeping, path) {
			return true
		}
	}

	return false
}

// alreadyAdjusted is what this repository's saved answer adjusts.
//
// Nothing where there is no row, or where the row holds a document this
// version cannot read. That is the safe direction: the save is then checked as
// though every adjustment in it were new.
func (s *Server) alreadyAdjusted(
	r *http.Request,
	targetID, repositoryID string,
) []string {
	stored, err := s.store.GetSyncRepositoryOverride(
		r.Context(), targetID, repositoryID, orgsync.KindFiles)
	if err != nil {
		return nil
	}

	var saved orgsync.FileOverride
	if err := json.Unmarshal(stored.Document, &saved); err != nil {
		return nil
	}

	return saved.Adjusted()
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

	document, readable := readableDocument(dto.Document)
	dto.Document, dto.Unreadable = document, !readable

	return dto
}
