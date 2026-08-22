package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
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

// syncPathDTO is one path and how many of this installation's repositories
// already hold it.
//
// The count is the whole point of aggregating: the same file across twenty-five
// repositories is one thing being configured, not twenty-five facts, and a
// finder listing it once per repository would be a finder nobody could read.
type syncPathDTO struct {
	Path         string `json:"path"`
	Repositories int    `json:"repositories"`
}

// heldPathIndex is one installation's aggregated path list, and the reading of
// the stored rows it was built from.
//
// Held rather than rebuilt because building it is the expensive part: the union
// of two hundred repositories is up to ten million map operations and a sort of
// a couple of hundred thousand entries, and the rows behind it change about
// once a day. `stamp` is what says whether they have - see pathIndexStamp.
type heldPathIndex struct {
	stamp  string
	answer map[string]any
}

// pathIndexStamp fingerprints the stored rows without reading a single path.
//
// Every field a scan carries, which is exactly what changes when a list is
// rewritten: a repository appears or goes, a tree is read at a new commit, a
// row is stamped as still current, or GitHub's truncation verdict moves. The
// paths themselves cannot change without the commit changing, because that is
// the whole premise of the refresh - so a fingerprint that never reads them
// still catches every rewrite.
func pathIndexStamp(scans []orgsync.RepositoryPathScan) string {
	digest := sha256.New()
	for _, scan := range scans {
		// A hash's Write never fails, which is why the error is dropped rather
		// than carried up through a function that has nothing to report.
		_, _ = fmt.Fprintf(digest, "%s\x00%d\x00%s\x00%t\x00",
			scan.RepositoryID, scan.ObservedAt.UnixNano(), scan.HeadSHA, scan.Partial)
	}

	return hex.EncodeToString(digest.Sum(nil))
}

// listSyncPaths answers with every path this installation's repositories are
// known to hold.
//
// Shipped whole and matched in the browser: it is a list this installation
// already has, it changes about once a day, and a request per keystroke to
// filter it would be a request per keystroke.
//
// A picture rather than a fact - whatever each default branch held when it was
// last looked at. The panel says so, and offers a path nobody holds yet anyway.
//
// Aggregated once per version of the rows rather than once per request. The
// cheap scan read decides: it carries no paths, so asking whether anything has
// changed costs a few hundred bytes where answering from scratch costs the
// whole index. A held answer is immutable and handed out as it is - nothing
// writes into it after it is built.
func (s *Server) listSyncPaths(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}

	scans, err := s.store.ListSyncRepositoryPathScans(r.Context(), target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	stamp := pathIndexStamp(scans)
	if held, found := s.pathIndex.Load(target.ID); found {
		if cached, sound := held.(heldPathIndex); sound && cached.stamp == stamp {
			writeJSON(w, http.StatusOK, cached.answer)

			return
		}
	}

	rows, err := s.store.ListSyncRepositoryPaths(r.Context(), target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	answer := syncPathIndex(rows)

	// Stamped with what was read BEFORE the aggregation, so a sweep that wrote
	// a row while this was building leaves a stamp that no longer matches and
	// the next reader rebuilds. Storing the stamp of a fresher read would pin a
	// stale answer to it.
	s.pathIndex.Store(target.ID, heldPathIndex{stamp: stamp, answer: answer})

	writeJSON(w, http.StatusOK, answer)
}

// syncPathIndex folds every repository's stored list into the one answer the
// finder reads.
//
// Its own function, and a pure one, because the panel is not the only thing
// that answers this address: the dev mock does too, and the two disagreed on
// the two fields nothing on screen shows plainly - what `repositories` counts,
// and which reading `observed_at` takes. Both are read by the notice above the
// finder, so a mock that got them wrong made the stale-index notice untestable
// in development and, in one direction, unreachable. `testdata/path-index.json`
// is the one table both sides are run against - the mechanism `filemerge`
// already uses for the composer.
func syncPathIndex(rows []orgsync.RepositoryPaths) map[string]any {
	counts := map[string]int{}
	var (
		observed time.Time
		partial  bool
	)
	for _, row := range rows {
		// The OLDEST, which is the same reading `partial` takes one line below:
		// this answer is the union of every repository's list, so how far it can
		// be trusted is decided by its weakest row rather than by its freshest.
		// The newest would say "checked a minute ago" for a list holding one
		// repository nothing has looked at in a week.
		if observed.IsZero() || row.ObservedAt.Before(observed) {
			observed = row.ObservedAt
		}
		// One repository GitHub would not finish listing makes the whole answer
		// some of what this installation holds. Said rather than left to look
		// like a short list that is complete.
		partial = partial || row.Partial
		for _, path := range row.Paths {
			counts[path]++
		}
	}

	paths := make([]syncPathDTO, 0, len(counts))
	for path, held := range counts {
		paths = append(paths, syncPathDTO{Path: path, Repositories: held})
	}
	// Held by most first, and by path where two are held by as many: the finder
	// ranks by its own match, and this decides only which of two equal matches
	// a reader sees first. Name order after that, so the list never shuffles.
	slices.SortFunc(paths, func(left, right syncPathDTO) int {
		if left.Repositories != right.Repositories {
			return right.Repositories - left.Repositories
		}

		return strings.Compare(left.Path, right.Path)
	})

	// `repositories` counts the rows this was built FROM, not the installation's
	// repositories: it is the denominator under "held by 4 of 6", and counting
	// repositories nothing has ever looked at would put a ceiling there that no
	// path can reach.
	answer := map[string]any{
		"paths": paths, repositoriesKey: len(rows), "partial": partial,
	}
	if !observed.IsZero() {
		answer["observed_at"] = observed
	}

	return answer
}

// syncOverrideRowDTO is one repository's answer, in a list of all of them.
//
// The name travels with it because the caller is a page about a file rather
// than about a repository: "three repositories adjust renovate.json" is
// answered by this list, and answering it with ids would mean a request per row
// to turn each one back into a word.
type syncOverrideRowDTO struct {
	RepositoryID   string `json:"repository_id"`
	RepositoryName string `json:"repository_name"`

	syncOverrideDTO
}

// listSyncOverrides reads every repository's answer about one kind.
//
// One request rather than one per repository. The page that needs this is the
// one about a shared file, which asks "who adjusts this, and how" - a question
// about the whole installation that the per-repository endpoint can only answer
// by being asked two hundred times.
//
// Repositories the installation no longer holds are left out by the store's own
// join, so a name is always a repository somebody can still open.
func (s *Server) listSyncOverrides(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	kind, ok := s.syncKind(w, r)
	if !ok {
		return
	}

	overrides, err := s.store.ListSyncRepositoryOverrides(r.Context(), target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	repositories, err := s.store.ListRepositories(r.Context(), target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	names := make(map[string]string, len(repositories))
	for _, repository := range repositories {
		names[repository.ID] = repository.Name
	}

	rows := make([]syncOverrideRowDTO, 0, len(overrides))
	for _, override := range overrides {
		if override.Kind != kind {
			continue
		}
		name, known := names[override.RepositoryID]
		if !known {
			continue
		}
		rows = append(rows, syncOverrideRowDTO{
			RepositoryID:    override.RepositoryID,
			RepositoryName:  name,
			syncOverrideDTO: syncOverrideToDTO(kind, &override),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"overrides": rows})
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
