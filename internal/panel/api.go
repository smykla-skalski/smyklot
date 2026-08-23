package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// nullableBool keeps JSON field presence separate from its nullable value.
// Repository enablement uses null deliberately to mean "inherit", while an
// omitted field means the client sent an incomplete replacement document.
type nullableBool struct {
	Value   *bool
	Present bool
}

type nullableValue[T any] struct {
	Value   *T
	Present bool
}

func (value *nullableValue[T]) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Value = nil

		return nil
	}
	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = &decoded

	return nil
}

func (value *nullableBool) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Value = nil

		return nil
	}

	var decoded bool
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = &decoded

	return nil
}

type configMigrationActor struct {
	accountID        string
	elevationID      *string
	sessionTokenHash string
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	account, ok := s.requireViewer(w, r)
	if !ok {
		return
	}
	targets, err := s.store.ListTargets(r.Context(), account.ID, s.now().UTC())
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	user, err := s.store.GetPanelUser(r.Context(), account.ID)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewerDTO(user, len(targets)))
}

func (s *Server) getTargets(w http.ResponseWriter, r *http.Request) {
	account, ok := s.requireViewer(w, r)
	if !ok {
		return
	}
	targets, err := s.store.ListTargets(r.Context(), account.ID, s.now().UTC())
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	response := make([]targetResponse, 0, len(targets))
	for _, target := range targets {
		access, accessErr := s.store.ResolveTargetAccess(
			r.Context(), account.ID, target.ID, s.now().UTC(),
		)
		if accessErr != nil {
			s.writeInternal(w, accessErr)
			return
		}
		response = append(response, targetDTO(s.runtimeValues(), target, access))
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": response})
}

func (s *Server) postRootInstallationSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireSameOrigin(w, r) {
		return
	}
	if _, _, ok := s.requireRoot(w, r); !ok {
		return
	}
	targetIDs, err := s.catalog.SyncCatalog(r.Context())
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	s.wakePendingCIGates()
	writeJSON(w, http.StatusOK, map[string]any{"target_ids": targetIDs})
}

func (s *Server) getRepositories(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	page, err := parseRepositoryPage(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_repository_query", err.Error())
		return
	}
	repositories, err := s.store.ListRepositoryPage(r.Context(), target.ID, page)
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	target.RepositoryDefaultEnabled = repositories.RepositoryDefaultEnabled
	writeJSON(w, http.StatusOK, repositoryPageDTO(target, repositories))
}

func (s *Server) getRepository(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	repository, ok := s.repository(w, r, target)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, repositoryDetailDTO(s.runtimeValues(), target, repository))
}

// pathIndexOverride reads a refresh interval off the wire, keeping what is
// stored where the request does not mention it.
//
// Four handlers decode a settings request carrying this field - an
// installation's and one repository's, each from the panel and from the Root
// console - and each spelled the same eight lines. The field lives on the
// shared request struct and the check did not, so a fifth handler decoding
// either struct would have accepted it with nothing looking at it.
//
// `runtimeDuration` does the work: it range-checks the seconds BEFORE
// multiplying, which is the whole difficulty - a Duration is int64 nanoseconds,
// so 18446744074 seconds multiplies to exactly 0, and 0 is "check every sweep",
// the most expensive answer there is.
func pathIndexOverride(
	stored *time.Duration,
	sent nullableValue[int64],
) (*time.Duration, error) {
	if !sent.Present {
		return stored, nil
	}

	return runtimeDuration(sent.Value, 0, MaxPathIndexInterval, "file list refresh interval")
}

func pendingCIQuietDuration(seconds *int64) *time.Duration {
	if seconds == nil {
		return nil
	}
	if *seconds < 0 {
		invalid := -time.Second

		return &invalid
	}
	if *seconds > int64((24*time.Hour)/time.Second) {
		invalid := 24*time.Hour + time.Second

		return &invalid
	}
	duration := time.Duration(*seconds) * time.Second

	return &duration
}

// postRepositoryConfigMigrationReset lets an operator put a refused
// configuration migration back on the table.
//
// A refusal is durable and never expires, because a pull request somebody
// closed is a decision rather than a timeout. That leaves exactly one way back,
// and this is it - without which "declined" would be a state only a database
// edit could leave.
func (s *Server) postRepositoryConfigMigrationReset(w http.ResponseWriter, r *http.Request) {
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
	s.resetConfigMigration(
		w, r, target, repository,
		configMigrationActor{accountID: account.ID},
		s.writeStorageError,
	)
}

// resetConfigMigration is the half of the reset that does not depend on how the
// caller proved they may do it. The two routes differ only in that, and having
// one body is what stops the root one drifting into a second set of rules.
func (s *Server) resetConfigMigration(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
	repository storage.Repository,
	actor configMigrationActor,
	writeError func(http.ResponseWriter, error),
) {
	if err := s.store.SetRepositoryConfigMigration(
		r.Context(),
		storage.RepositoryConfigMigration{
			TargetID:         target.ID,
			RepositoryID:     repository.ID,
			State:            storage.ConfigMigrationNone,
			ActorAccountID:   &actor.accountID,
			ElevationID:      actor.elevationID,
			SessionTokenHash: actor.sessionTokenHash,
			ChangedAt:        s.now().UTC(),
		},
	); err != nil {
		writeError(w, err)
		return
	}
	updated, err := s.store.GetRepository(r.Context(), target.ID, repository.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !s.attachPendingCIGate(w, r, &updated) {
		return
	}
	s.Announce(target.ID, updated.ID)
	writeJSON(w, http.StatusOK, repositoryDetailDTO(s.runtimeValues(), target, updated))
}

func (s *Server) repository(
	w http.ResponseWriter,
	r *http.Request,
	target storage.Target,
) (storage.Repository, bool) {
	repository, err := s.store.GetRepository(
		r.Context(),
		target.ID,
		r.PathValue("repository"),
	)
	if err != nil {
		s.writeStorageError(w, err)
		return storage.Repository{}, false
	}
	if !repository.Available {
		s.writeError(w, http.StatusNotFound, "not_found", "repository not found")
		return storage.Repository{}, false
	}
	if !s.attachPendingCIGate(w, r, &repository) {
		return storage.Repository{}, false
	}

	return repository, true
}

func (s *Server) attachPendingCIGate(
	w http.ResponseWriter,
	r *http.Request,
	repository *storage.Repository,
) bool {
	gate, err := s.store.GetPendingCIRepositoryGate(r.Context(), repository.ID)
	if err != nil {
		s.writeStorageError(w, err)
		return false
	}
	repository.PendingCIGate = &gate

	return true
}

func (s *Server) getAudit(w http.ResponseWriter, r *http.Request) {
	target, ok := s.historyTarget(w, r)
	if !ok {
		return
	}
	s.getInstallationAuditPage(w, r, target.ID)
}

func (s *Server) getInstallationAuditPage(w http.ResponseWriter, r *http.Request, targetID string) {
	page, err := parseHistoryPage(r.URL.Query(), auditHistoryOrders...)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", err.Error())
		return
	}
	scope := storage.AuditScope(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = storage.AuditAll
	}
	if scope != storage.AuditAll && scope != storage.AuditAccount &&
		scope != storage.AuditRepositories {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", "invalid audit scope")
		return
	}
	change := storage.AuditChange(r.URL.Query().Get("change"))
	if change == "" {
		change = storage.AuditChangeAll
	}
	if change != storage.AuditChangeAll && change != storage.AuditChangeRepository &&
		change != storage.AuditChangeAccount &&
		change != storage.AuditChangeSync {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", "invalid audit change")
		return
	}
	result, err := s.store.ListAudit(r.Context(), targetID, storage.AuditPageRequest{
		HistoryPageRequest: page,
		Scope:              scope,
		Change:             change,
	})
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, auditPageDTO(result))
}

func (s *Server) getFailures(w http.ResponseWriter, r *http.Request) {
	target, ok := s.historyTarget(w, r)
	if !ok {
		return
	}
	s.getInstallationFailurePage(w, r, target.ID)
}

func (s *Server) getInstallationFailurePage(w http.ResponseWriter, r *http.Request, targetID string) {
	page, err := parseHistoryPage(r.URL.Query(), failureHistoryOrders...)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", err.Error())
		return
	}
	var retryable *bool
	switch r.URL.Query().Get("kind") {
	case "", allFilter:
	case "retryable":
		value := true
		retryable = &value
	case "permanent":
		value := false
		retryable = &value
	default:
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", "invalid failure kind")
		return
	}
	result, err := s.store.ListFailures(r.Context(), targetID, storage.FailurePageRequest{
		HistoryPageRequest: page,
		Retryable:          retryable,
	})
	if err != nil {
		s.writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, failurePageDTO(result))
}

func (s *Server) historyTarget(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Target, bool) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return storage.Target{}, false
	}

	return target, true
}
