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

	s.answerSyncOverride(w, r, target.ID, repository.ID, kind, saved)
}

// answerSyncOverride answers with what this repository says about one kind and
// what the planner last made of it.
//
// Two rows and one question. The override is what somebody asked for; the state
// row is what came of it, and a pane showing only the first would show a
// repository that looks configured and is being skipped.
func (s *Server) answerSyncOverride(
	w http.ResponseWriter,
	r *http.Request,
	targetID, repositoryID string,
	kind orgsync.Kind,
	override *orgsync.RepositoryOverride,
) {
	updatedBy := ""
	if override != nil {
		var err error
		updatedBy, err = s.syncEditorLogin(r.Context(), override.UpdatedBy)
		if err != nil {
			s.writeStorageError(w, err)

			return
		}
	}
	dto := syncOverrideToDTO(kind, override, updatedBy)

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
func syncOverrideToDTO(
	kind orgsync.Kind,
	override *orgsync.RepositoryOverride,
	updatedBy string,
) syncOverrideDTO {
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
		UpdatedBy: updatedBy,
		UpdatedAt: &override.UpdatedAt,
	}

	document, readable := readableDocument(dto.Document)
	dto.Document, dto.Unreadable = document, !readable

	return dto
}
