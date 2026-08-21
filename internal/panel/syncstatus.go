package panel

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

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

	configs, err := s.store.ListSyncConfigs(ctx, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}
	enabledKinds := make(map[orgsync.Kind]bool, len(configs))
	for _, config := range configs {
		enabledKinds[config.Kind] = config.Enabled
	}

	overrides, err := s.store.ListSyncRepositoryOverrides(ctx, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}
	answered := make(map[string]map[orgsync.Kind]*bool, len(overrides))
	for _, override := range overrides {
		if answered[override.RepositoryID] == nil {
			answered[override.RepositoryID] = map[orgsync.Kind]*bool{}
		}
		answered[override.RepositoryID][override.Kind] = override.Enabled
	}

	states, err := s.store.ListSyncRepositoryState(ctx, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}
	problems := make(map[string]map[orgsync.Kind]string, len(states))
	var checked time.Time
	for _, state := range states {
		if state.AppliedAt.After(checked) {
			checked = state.AppliedAt
		}
		if state.Problem == "" {
			continue
		}
		if problems[state.RepositoryID] == nil {
			problems[state.RepositoryID] = map[orgsync.Kind]string{}
		}
		problems[state.RepositoryID][state.Kind] = state.Problem
	}

	// Pending counts come off the live plan, which is the only account of what
	// would change. No plan is an answer: nothing is pending anywhere.
	pending := map[string]map[orgsync.Kind]int{}
	removals := map[string]int{}
	plan, actions, err := s.store.GetLiveSyncPlan(ctx, target.ID)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		// Nothing in flight.
	case err != nil:
		s.writeStorageError(w, err)

		return
	default:
		if plan.ComputedAt.After(checked) {
			checked = plan.ComputedAt
		}
		for _, action := range actions {
			if pending[action.RepositoryID] == nil {
				pending[action.RepositoryID] = map[orgsync.Kind]int{}
			}
			pending[action.RepositoryID][action.Kind]++
			if action.Operation == orgsync.OperationDelete {
				removals[action.RepositoryID]++
			}
		}
	}

	if checked.IsZero() {
		// Nothing has been looked at yet - a fresh installation. The board is
		// still an answer, dated to the moment it was composed.
		checked = s.now().UTC()
	}

	rows := make([]syncRepositoryStatusDTO, 0, len(repositories))
	for _, repository := range repositories {
		row := syncRepositoryStatusDTO{
			Repository: repository.Name,
			Cells:      make(map[string]syncCellDTO, len(orgsync.Kinds())),
		}
		for _, kind := range orgsync.Kinds() {
			enabled := enabledKinds[kind]
			if answer := answered[repository.ID][kind]; answer != nil {
				enabled = *answer
			}

			switch {
			case !enabled:
				row.Cells[string(kind)] = syncCellDTO{State: "off"}
			case problems[repository.ID][kind] != "":
				row.Cells[string(kind)] = syncCellDTO{State: "refused"}
				if row.Reason == "" {
					row.Reason = problems[repository.ID][kind]
				}
			case pending[repository.ID][kind] > 0:
				row.Cells[string(kind)] = syncCellDTO{
					State: "pending", Changes: pending[repository.ID][kind],
				}
			default:
				row.Cells[string(kind)] = syncCellDTO{State: "in_step"}
			}
		}
		row.Removals = removals[repository.ID]
		rows = append(rows, row)
	}

	sort.Slice(rows, func(left, right int) bool {
		return rows[left].Repository < rows[right].Repository
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"checked_at":   checked,
		"repositories": rows,
	})
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
		if override.Enabled != nil && !*override.Enabled && filesEnabled {
			covered--
		}
		if len(override.Document) == 0 {
			continue
		}

		// The document's own spelling: a list of merges, each naming its path.
		// Decoded loosely on purpose - the stored merge travels to the browser
		// whole, and a shape this version does not know is still worth showing.
		var document struct {
			Merges []json.RawMessage `json:"merges"`
		}
		if err := json.Unmarshal(override.Document, &document); err != nil {
			continue
		}
		for _, merge := range document.Merges {
			var named struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(merge, &named); err != nil || named.Path == "" {
				continue
			}
			merges = append(merges, syncFileMergeEntryDTO{
				Repository:   name,
				RepositoryID: override.RepositoryID,
				Path:         named.Path,
				Merge:        merge,
			})
		}
	}

	rows, err := s.store.ListSyncRepositoryPaths(ctx, target.ID)
	if err != nil {
		s.writeStorageError(w, err)

		return
	}
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
	knownPaths := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		knownPaths = append(knownPaths, map[string]any{
			"path": path, "repositories": counts[path],
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"repositories": len(repositories),
		"covered":      covered,
		"known_paths":  knownPaths,
		"merges":       merges,
	})
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

	writeJSON(w, http.StatusOK, map[string]any{syncPlanKey: syncPlanToDTO(plan, nil)})
}
