package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// repositoriesKey is the wire name three sync answers share for their
// repository list or count.
const repositoriesKey = "repositories"

// syncCellDTO is where one repository stands for one kind: settled, waiting on
// a plan, refused with a reason a person can act on, or switched off there.
type syncCellDTO struct {
	State string `json:"state"`

	// Changes is pending only: how many of the plan's changes land here.
	Changes int `json:"changes,omitempty"`
}

// syncRepositoryStatusDTO is one repository on the overview's board.
type syncRepositoryStatusDTO struct {
	Repository string                 `json:"repository"`
	Cells      map[string]syncCellDTO `json:"cells"`

	// Removals is pending only: how many of the changes are removals.
	Removals int `json:"removals,omitempty"`

	// Reason is refused only: the planner's own words about why.
	Reason string `json:"reason,omitempty"`
}

// syncStatusFacts is everything getSyncStatus reads, keyed the way the row
// builder asks: by repository, then by kind.
type syncStatusFacts struct {
	enabled  map[orgsync.Kind]bool
	answered map[string]map[orgsync.Kind]*bool
	problems map[string]map[orgsync.Kind]string
	pending  map[string]map[orgsync.Kind]int
	removals map[string]int
	checked  time.Time
}

// getSyncStatus answers the fleet: every repository sync covers and where each
// one stands, per kind. The overview's board, legend, out-of-step list and
// kind strips are all drawn from this one read.
//
// Composed here rather than stored: everything it says is already in the
// configuration, the per-repository state rows and the live plan, and a stored
// copy would be one more thing a sweep has to keep true.
func (s *Server) getSyncStatus(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}

	ctx := r.Context()

	repositories, err := s.store.ListRepositories(ctx, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	facts, err := s.syncStatusFacts(r, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	if facts.checked.IsZero() {
		// Nothing has been looked at yet - a fresh installation. The board is
		// still an answer, dated to the moment it was composed.
		facts.checked = s.now().UTC()
	}

	rows := make([]syncRepositoryStatusDTO, 0, len(repositories))
	for _, repository := range repositories {
		rows = append(rows, syncStatusRow(repository, facts))
	}
	sort.Slice(rows, func(left, right int) bool {
		return rows[left].Repository < rows[right].Repository
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"checked_at":    facts.checked,
		repositoriesKey: rows,
	})
}

// syncStatusFacts gathers what the board says from the three places it lives:
// the configuration, the per-repository state rows, and the live plan.
func (s *Server) syncStatusFacts(r *http.Request, targetID string) (syncStatusFacts, error) {
	ctx := r.Context()
	facts := syncStatusFacts{
		answered: map[string]map[orgsync.Kind]*bool{},
		problems: map[string]map[orgsync.Kind]string{},
		pending:  map[string]map[orgsync.Kind]int{},
		removals: map[string]int{},
	}

	configs, err := s.store.ListSyncConfigs(ctx, targetID)
	if err != nil {
		return facts, err
	}
	facts.enabled = make(map[orgsync.Kind]bool, len(configs))
	for _, config := range configs {
		facts.enabled[config.Kind] = config.Enabled
	}

	overrides, err := s.store.ListSyncRepositoryOverrides(ctx, targetID)
	if err != nil {
		return facts, err
	}
	for _, override := range overrides {
		if facts.answered[override.RepositoryID] == nil {
			facts.answered[override.RepositoryID] = map[orgsync.Kind]*bool{}
		}
		facts.answered[override.RepositoryID][override.Kind] = override.Enabled
	}

	states, err := s.store.ListSyncRepositoryState(ctx, targetID)
	if err != nil {
		return facts, err
	}
	for _, state := range states {
		if state.AppliedAt.After(facts.checked) {
			facts.checked = state.AppliedAt
		}
		if state.Problem == "" {
			continue
		}
		if facts.problems[state.RepositoryID] == nil {
			facts.problems[state.RepositoryID] = map[orgsync.Kind]string{}
		}
		facts.problems[state.RepositoryID][state.Kind] = state.Problem
	}

	return facts, s.pendingSyncFacts(ctx, targetID, &facts)
}

// pendingSyncFacts folds the live plan into the facts. Pending counts come off
// the plan, which is the only account of what would change - and no plan is an
// answer: nothing is pending anywhere.
func (s *Server) pendingSyncFacts(
	ctx context.Context, targetID string, facts *syncStatusFacts,
) error {
	plan, actions, err := s.store.GetLiveSyncPlan(ctx, targetID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if plan.ComputedAt.After(facts.checked) {
		facts.checked = plan.ComputedAt
	}
	for _, action := range actions {
		if facts.pending[action.RepositoryID] == nil {
			facts.pending[action.RepositoryID] = map[orgsync.Kind]int{}
		}
		facts.pending[action.RepositoryID][action.Kind]++
		if action.Operation == orgsync.OperationDelete {
			facts.removals[action.RepositoryID]++
		}
	}

	return nil
}

// syncStatusRow reads one repository's standing off the gathered facts.
func syncStatusRow(repository storage.Repository, facts syncStatusFacts) syncRepositoryStatusDTO {
	row := syncRepositoryStatusDTO{
		Repository: repository.Name,
		Cells:      make(map[string]syncCellDTO, len(orgsync.Kinds())),
	}
	for _, kind := range orgsync.Kinds() {
		enabled := facts.enabled[kind]
		if answer := facts.answered[repository.ID][kind]; answer != nil {
			enabled = *answer
		}

		switch {
		case !enabled:
			row.Cells[string(kind)] = syncCellDTO{State: "off"}
		case facts.problems[repository.ID][kind] != "":
			row.Cells[string(kind)] = syncCellDTO{State: "refused"}
			if row.Reason == "" {
				row.Reason = facts.problems[repository.ID][kind]
			}
		case facts.pending[repository.ID][kind] > 0:
			row.Cells[string(kind)] = syncCellDTO{
				State: "pending", Changes: facts.pending[repository.ID][kind],
			}
		default:
			row.Cells[string(kind)] = syncCellDTO{State: "in_step"}
		}
	}
	row.Removals = facts.removals[repository.ID]

	return row
}

// syncFileMergeEntryDTO is one repository's adjustment of one template.
type syncFileMergeEntryDTO struct {
	Repository   string          `json:"repository"`
	RepositoryID string          `json:"repository_id"`
	Path         string          `json:"path"`
	Merge        json.RawMessage `json:"merge"`
}

// getSyncFilesContext answers what the files pages need beyond the document:
// the path index the finder matches over, and every repository adjustment of
// every template, so the list can count adjusters and the file page can show
// them.
func (s *Server) getSyncFilesContext(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}

	ctx := r.Context()

	repositories, err := s.store.ListRepositories(ctx, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}
	names := make(map[string]string, len(repositories))
	for _, repository := range repositories {
		names[repository.ID] = repository.Name
	}

	config, err := s.store.GetSyncConfig(ctx, target.ID, orgsync.KindFiles)
	filesEnabled := err == nil && config.Enabled
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.writeStorageError(w, err)

		return
	}

	overrides, err := s.store.ListSyncRepositoryOverrides(ctx, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	covered := 0
	if filesEnabled {
		covered = len(repositories)
	}
	merges := []syncFileMergeEntryDTO{}
	for _, override := range overrides {
		if override.Kind != orgsync.KindFiles {
			continue
		}
		name, known := names[override.RepositoryID]
		if !known {
			continue
		}
		covered += syncCoverageDelta(filesEnabled, override.Enabled)
		merges = append(merges, fileMergeEntries(name, override)...)
	}

	rows, err := s.store.ListSyncRepositoryPaths(ctx, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		repositoriesKey: len(repositories),
		"covered":       covered,
		"known_paths":   knownSyncPaths(rows),
		"merges":        merges,
	})
}

// syncCoverageDelta adjusts a baseline count when a repository explicitly
// answers differently from the installation.
func syncCoverageDelta(defaultEnabled bool, override *bool) int {
	if override == nil || *override == defaultEnabled {
		return 0
	}
	if *override {
		return 1
	}

	return -1
}

// fileMergeEntries reads one repository's adjustments out of its override
// document.
//
// The document's own spelling: a list of merges, each naming its path.
// Decoded loosely on purpose - the stored merge travels to the browser whole,
// and a shape this version does not know is still worth showing.
func fileMergeEntries(name string, override orgsync.RepositoryOverride) []syncFileMergeEntryDTO {
	if len(override.Document) == 0 {
		return nil
	}
	var document struct {
		Merges []json.RawMessage `json:"merges"`
	}
	if err := json.Unmarshal(override.Document, &document); err != nil {
		return nil
	}

	entries := make([]syncFileMergeEntryDTO, 0, len(document.Merges))
	for _, merge := range document.Merges {
		var named struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(merge, &named); err != nil || named.Path == "" {
			continue
		}
		entries = append(entries, syncFileMergeEntryDTO{
			Repository:   name,
			RepositoryID: override.RepositoryID,
			Path:         named.Path,
			Merge:        merge,
		})
	}

	return entries
}

// knownSyncPaths folds the per-repository path rows into the finder's index:
// every path anything holds, and how many repositories hold it.
func knownSyncPaths(rows []orgsync.RepositoryPaths) []map[string]any {
	counts := map[string]int{}
	for _, row := range rows {
		for _, path := range row.Paths {
			counts[path]++
		}
	}
	paths := make([]string, 0, len(counts))
	for path := range counts {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	known := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		known = append(known, map[string]any{
			"path": path, repositoriesKey: counts[path],
		})
	}

	return known
}

// deleteSyncPlan retires a live plan somebody read and declined.
//
// Discarding asks nothing on GitHub: the plan leaves the live slot and the
// next sweep computes a fresh one from whatever the configuration says by
// then. Which is why, unlike an approval, no digest travels - declining a plan
// that changed underneath the reader declines the newer one just as well.
func (s *Server) deleteSyncPlan(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	account, target, _, ok := s.requireTarget(w, r, true)
	if !ok {
		return
	}

	plan, err := s.store.DiscardSyncPlan(r.Context(), orgsync.PlanDiscard{
		TargetID: target.ID,
		PlanID:   r.PathValue(syncPlanKey),
		ActorID:  account.ID,
		Now:      s.now().UTC(),
	})
	if errors.Is(err, storage.ErrConflict) {
		// The one refusal worth its own message: the plan left the reader's
		// hands - an executor holds it, or it already finished.
		s.writeError(w, http.StatusConflict, "plan_not_live",
			"this plan is no longer waiting; ask for a fresh one")

		return
	}
	if err != nil {
		s.writeStorageError(w, err)

		return
	}

	if err := s.store.RecordSyncAudit(r.Context(), orgsync.AuditEntry{
		TargetID: target.ID, PlanID: plan.ID, ActorID: account.ID,
		Action:  orgsync.AuditDiscarded,
		Summary: "discarded a plan of " + syncApprovalSummary(plan.Counts),
		Counts:  plan.Counts,
		Now:     s.now().UTC(),
	}); err != nil {
		s.writeInternal(w, err)

		return
	}

	s.Announce(target.ID, "")

	writeJSON(w, http.StatusOK, map[string]any{syncPlanKey: syncPlanToDTO(plan, nil, nil)})
}
