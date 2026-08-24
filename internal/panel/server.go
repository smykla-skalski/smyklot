package panel

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

const maxRequestBody = 64 << 10

// maxDocumentBody bounds a request carrying templates. See bodyBoundFor, which
// is the only thing that hands it out.
//
// Well above the megabyte FileConfig allows rather than at it, because a
// megabyte of file content is more than a megabyte of JSON: every newline,
// quote and backslash in it is escaped on the way.
const maxDocumentBody = 4 << 20

// bodyBoundFor is how large a request carrying one kind's configuration may be.
//
// The larger bound only where something validates against a limit that needs
// it. Files do: FileConfig allows a megabyte of templates in total, and a cap
// below that makes the documented limit unreachable through the only writer
// there is - a dozen medium workflow files pasted into the shared-files form
// were truncated at 64 KiB and refused as invalid JSON, which sends whoever
// pasted them looking for a syntax error in a YAML file that has none.
//
// No other kind has a total of its own. A label document bounds each name,
// colour and description and not how many there are, so raising the bound for
// them raises nothing but the size of a mistake - and a label document is an
// action per label per repository once it is planned.
func bodyBoundFor(kind orgsync.Kind) int64 {
	if kind == orgsync.KindFiles {
		return maxDocumentBody
	}

	return maxRequestBody
}

type catalogSyncer interface {
	SyncCatalog(context.Context) ([]string, error)
}

type userResolver interface {
	ResolveUser(context.Context, string, string) (storage.Account, error)
	ResolveRootUser(context.Context, string) (storage.Account, error)
}

// RuntimeController applies validated effective values to the long-running
// service. The operation is infallible because every rejected value is stopped
// before persistence.
type RuntimeController interface {
	ApplyRuntimeSettings(RuntimeValues)
}

// PendingCIController applies operator transitions without exposing service
// coordination or reconciliation policy to the panel.
type PendingCIController interface {
	CheckNow(context.Context, pendingci.CheckNowRequest) (pendingci.Request, error)
	Cancel(context.Context, pendingci.FinishRequest) (pendingci.Request, error)
	Exclusive(context.Context, []string, func() error) error
	ExclusiveCatalog(context.Context, func() error) error
	Wake()
}

type PendingCIGateController interface {
	WakePendingCIGates()
}

type WorkQueueController interface {
	WakeQueue(workqueue.Lane)
}

// Dependencies are the service capabilities used by panel handlers.
type Dependencies struct {
	Store     storage.Store
	Catalog   catalogSyncer
	Users     userResolver
	SignIn    signInProvider
	Random    io.Reader
	Now       func() time.Time
	Runtime   RuntimeController
	PendingCI PendingCIController
	Gates     PendingCIGateController
	Queue     WorkQueueController
	// Candidates reads the roster logins are completed against. Optional: a
	// panel without one offers no completion, which is what the dialogs did
	// before there was any.
	Candidates candidateDirectory
}

// Server owns the panel routes and their authenticated runtime state.
type Server struct {
	cfg        Config
	store      storage.Store
	catalog    catalogSyncer
	users      userResolver
	candidates candidateDirectory
	signIn     signInProvider
	random     io.Reader
	now        func() time.Time
	startedAt  time.Time
	assets     *assetBundle
	events     *eventHub
	runtimeMu  sync.RWMutex
	runtime    RuntimeValues
	controller RuntimeController
	pendingCI  PendingCIController
	gates      PendingCIGateController
	queue      WorkQueueController
	// prefsMu spans each preference commit and its fan-out so announce order
	// matches commit order (see applyPrefsPatch).
	prefsMu sync.Mutex

	// pathIndex holds one aggregated path list per installation, keyed by a
	// fingerprint of the rows it was built from (see pathIndexStamp).
	//
	// A `sync.Map` because the shape fits it exactly: written about once a day
	// per installation, read on every finder open, and never iterated. One
	// entry per installation this process has served, which is bounded by the
	// installations it has - and each holds paths this process was going to
	// send anyway.
	pathIndex sync.Map
}

// New creates a production panel server.
func New(cfg Config, deps Dependencies) (*Server, error) {
	validated, err := cfg.validated()
	if err != nil {
		return nil, err
	}
	if deps.Store == nil || deps.Catalog == nil || deps.Users == nil || deps.PendingCI == nil {
		return nil, fmt.Errorf(
			"%w: storage, catalog sync, user lookup, and pending CI control are required",
			errInvalidConfig,
		)
	}
	if deps.SignIn == nil {
		deps.SignIn, err = newGitHubSignIn(validated)
		if err != nil {
			return nil, err
		}
	}
	if deps.Random == nil {
		deps.Random = rand.Reader
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	assets, err := newAssetBundle(validated)
	if err != nil {
		return nil, err
	}

	persisted, err := deps.Store.GetRuntimeSettings(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load runtime settings: %w", err)
	}
	runtime, err := resolveRuntimeValues(validated, persisted)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime settings: %w", err)
	}
	if err := deps.Store.InitializeQueuePolicies(
		context.Background(), queueDeploymentDefaults(runtime), deps.Now().UTC(),
	); err != nil {
		return nil, fmt.Errorf("initialize queue policies: %w", err)
	}
	if deps.Runtime != nil {
		deps.Runtime.ApplyRuntimeSettings(runtime)
	}

	return &Server{
		cfg:        validated,
		store:      deps.Store,
		catalog:    deps.Catalog,
		users:      deps.Users,
		candidates: deps.Candidates,
		signIn:     deps.SignIn,
		random:     deps.Random,
		now:        deps.Now,
		startedAt:  deps.Now().UTC(),
		assets:     assets,
		events:     newEventHub(),
		runtime:    runtime,
		controller: deps.Runtime,
		pendingCI:  deps.PendingCI,
		gates:      deps.Gates,
		queue:      deps.Queue,
	}, nil
}

func (s *Server) wakePendingCIGates() {
	if s.gates != nil {
		s.gates.WakePendingCIGates()
	}
}

// Handler returns the complete panel HTTP surface at its configured base path.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	base := s.cfg.BasePath

	mux.HandleFunc("GET "+base+"/auth/github/start", s.startSignIn)
	mux.HandleFunc("GET "+base+"/auth/github/callback", s.finishSignIn)
	mux.HandleFunc("POST "+base+"/api/v1/sign-out", s.signOut)
	mux.HandleFunc("GET "+base+"/api/v1/session", s.getSession)
	mux.HandleFunc("GET "+base+"/api/v1/targets", s.getTargets)
	mux.HandleFunc("GET "+base+"/api/v1/notifications", s.getSecurityNotifications)
	mux.HandleFunc(
		"PUT "+base+"/api/v1/notifications/{notification}/read",
		s.putSecurityNotificationRead,
	)
	mux.HandleFunc("GET "+base+"/api/v1/invites/{token}", s.reviewInvitation)
	s.registerRootRoutes(mux, base)
	s.registerInstallationSettingsRoutes(mux, base)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/users", s.getTargetUsers)
	mux.HandleFunc(
		"GET "+base+"/api/v1/targets/{target}/user-suggestions",
		s.getTargetUserSuggestions,
	)
	mux.HandleFunc("POST "+base+"/api/v1/targets/{target}/users", s.postTargetUser)
	mux.HandleFunc(
		"GET "+base+"/api/v1/targets/{target}/users/{account}/decisions",
		s.getTargetUserDecisions,
	)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/invitations", s.getTargetInvitations)
	mux.HandleFunc("POST "+base+"/api/v1/targets/{target}/invitations", s.postTargetInvitation)
	mux.HandleFunc(
		"POST "+base+"/api/v1/targets/{target}/invitations/{invitation}/reissue",
		s.reissueInvitation,
	)
	mux.HandleFunc(
		"DELETE "+base+"/api/v1/targets/{target}/invitations/{invitation}",
		s.deleteInvitation,
	)
	mux.HandleFunc(
		"PUT "+base+"/api/v1/targets/{target}/users/{account}",
		s.putTargetUser,
	)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/repositories", s.getRepositories)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/repositories/{repository}", s.getRepository)
	mux.HandleFunc(
		"POST "+base+"/api/v1/targets/{target}/repositories/{repository}/config-migration",
		s.postRepositoryConfigMigrationReset,
	)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/config/{kind}", s.getSyncConfig)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/paths", s.listSyncPaths)
	mux.HandleFunc(
		"GET "+base+"/api/v1/targets/{target}/repositories/{repository}/sync/{kind}",
		s.getSyncOverride,
	)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/plan", s.getSyncPlan)
	mux.HandleFunc("POST "+base+"/api/v1/targets/{target}/sync/run-now", s.postSyncRunNow)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/sync/status", s.getSyncStatus)
	mux.HandleFunc(
		"GET "+base+"/api/v1/targets/{target}/sync/files/context",
		s.getSyncFilesContext,
	)
	mux.HandleFunc(
		"DELETE "+base+"/api/v1/targets/{target}/sync/plans/{plan}",
		s.deleteSyncPlan,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/targets/{target}/sync/plans/{plan}/approval",
		s.postSyncPlanApproval,
	)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/audit", s.getAudit)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/failures", s.getFailures)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/queue", s.getTargetQueue)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/queue/{queue}", s.getTargetQueueItem)
	mux.HandleFunc("POST "+base+"/api/v1/targets/{target}/queue/{queue}/actions/preview", s.previewTargetQueueAction)
	mux.HandleFunc("POST "+base+"/api/v1/targets/{target}/queue/{queue}/actions", s.postTargetQueueAction)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/schedules", s.getTargetSchedules)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/schedule-requests", s.getTargetScheduleRequests)
	mux.HandleFunc("POST "+base+"/api/v1/targets/{target}/schedule-requests", s.postTargetScheduleRequest)
	mux.HandleFunc("DELETE "+base+"/api/v1/targets/{target}/schedule-requests/{request}", s.deleteTargetScheduleRequest)
	mux.HandleFunc("GET "+base+"/api/v1/events", s.streamEvents)

	if base != "" {
		mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, base+"/", http.StatusPermanentRedirect)
		})
	}
	mux.HandleFunc("GET "+base+"/", s.serveAsset)

	return s.secureHeaders(mux)
}

func (s *Server) registerInstallationSettingsRoutes(mux *http.ServeMux, base string) {
	mux.HandleFunc(
		"PUT "+base+"/api/v1/targets/{target}/settings",
		s.putInstallationSettingsBatch,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/targets/{target}/settings/checkpoints/baseline",
		s.getInstallationSettingsBaseline,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/targets/{target}/settings/checkpoints/{checkpoint}",
		s.getInstallationSettingsCheckpoint,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/targets/{target}/settings/checkpoints/{checkpoint}/restore",
		s.postInstallationSettingsRestore,
	)
}

func (s *Server) registerRootRoutes(mux *http.ServeMux, base string) {
	s.registerRootQueueScheduleRoutes(mux, base)
	mux.HandleFunc("GET "+base+"/api/v1/root/installations", s.getRootInstallations)
	mux.HandleFunc("GET "+base+"/api/v1/root/overview", s.getRootOverview)
	mux.HandleFunc("GET "+base+"/api/v1/root/pending-ci/{request}", s.getRootPendingCI)
	mux.HandleFunc("POST "+base+"/api/v1/root/pending-ci/{request}/check", s.postRootPendingCICheck)
	mux.HandleFunc("DELETE "+base+"/api/v1/root/pending-ci/{request}", s.deleteRootPendingCI)
	mux.HandleFunc("GET "+base+"/api/v1/root/runtime/settings", s.getRootRuntimeSettings)
	mux.HandleFunc("PUT "+base+"/api/v1/root/runtime/settings", s.putRootRuntimeSettings)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/runtime/settings/checkpoints/baseline",
		s.getRootSettingsBaseline,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/runtime/settings/checkpoints/{checkpoint}",
		s.getRootSettingsCheckpoint,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/root/runtime/settings/checkpoints/{checkpoint}/restore",
		s.postRootSettingsRestore,
	)
	mux.HandleFunc("GET "+base+"/api/v1/root/history/{history}", s.getRootHistory)
	mux.HandleFunc("GET "+base+"/api/v1/root/access/{access}", s.getRootAccess)
	mux.HandleFunc("PUT "+base+"/api/v1/root/access/users/{account}", s.putRootUser)
	mux.HandleFunc("POST "+base+"/api/v1/root/access/invitations", s.postRootInvitation)
	mux.HandleFunc(
		"POST "+base+"/api/v1/root/access/invitations/{invitation}/reissue",
		s.reissueRootInvitation,
	)
	mux.HandleFunc(
		"DELETE "+base+"/api/v1/root/access/invitations/{invitation}",
		s.deleteRootInvitation,
	)
	mux.HandleFunc("POST "+base+"/api/v1/root/installations/sync", s.postRootInstallationSync)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/elevation",
		s.getRootElevation,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/root/installations/{target}/elevation",
		s.postRootElevation,
	)
	mux.HandleFunc(
		"DELETE "+base+"/api/v1/root/elevations/{elevation}",
		s.deleteRootElevation,
	)
	s.registerRootInstallationSettingsRoutes(mux, base)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/repositories",
		s.getRootRepositories,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/repositories/{repository}",
		s.getRootRepository,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/root/installations/{target}/repositories/{repository}/config-migration",
		s.postRootRepositoryConfigMigrationReset,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/users",
		s.getRootTargetUsers,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/root/installations/{target}/users",
		s.postRootTargetUser,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/user-suggestions",
		s.getRootTargetUserSuggestions,
	)
	mux.HandleFunc(
		"PUT "+base+"/api/v1/root/installations/{target}/users/{account}",
		s.putRootTargetUser,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/users/{account}/decisions",
		s.getRootTargetUserDecisions,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/invitations",
		s.getRootTargetInvitations,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/root/installations/{target}/invitations",
		s.postRootTargetInvitation,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/root/installations/{target}/invitations/{invitation}/reissue",
		s.reissueRootTargetInvitation,
	)
	mux.HandleFunc(
		"DELETE "+base+"/api/v1/root/installations/{target}/invitations/{invitation}",
		s.deleteRootTargetInvitation,
	)
	s.registerRootTargetAuditRoute(mux, base)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/failures",
		s.getRootTargetFailures,
	)
}

func (s *Server) registerRootQueueScheduleRoutes(mux *http.ServeMux, base string) {
	mux.HandleFunc("GET "+base+"/api/v1/root/queue", s.getRootQueue)
	mux.HandleFunc("GET "+base+"/api/v1/root/queue/{queue}", s.getRootQueueItem)
	mux.HandleFunc("POST "+base+"/api/v1/root/queue/{queue}/actions/preview", s.previewRootQueueAction)
	mux.HandleFunc("POST "+base+"/api/v1/root/queue/{queue}/actions", s.postRootQueueAction)
	mux.HandleFunc("GET "+base+"/api/v1/root/schedule-profiles", s.getRootScheduleProfiles)
	mux.HandleFunc("POST "+base+"/api/v1/root/schedule-profiles", s.postRootScheduleProfile)
	mux.HandleFunc("PUT "+base+"/api/v1/root/schedule-profiles/{profile}", s.putRootScheduleProfile)
	mux.HandleFunc("DELETE "+base+"/api/v1/root/schedule-profiles/{profile}", s.deleteRootScheduleProfile)
	mux.HandleFunc("GET "+base+"/api/v1/root/job-policies", s.getRootJobPolicies)
	mux.HandleFunc("PUT "+base+"/api/v1/root/job-policies/{kind}", s.putRootJobPolicy)
	mux.HandleFunc("PUT "+base+"/api/v1/root/installations/{target}/job-policies/{kind}", s.putRootInstallationJobPolicy)
	mux.HandleFunc("DELETE "+base+"/api/v1/root/installations/{target}/job-policies/{kind}", s.deleteRootInstallationJobPolicy)
	mux.HandleFunc("GET "+base+"/api/v1/root/schedule-requests", s.getRootScheduleRequests)
	mux.HandleFunc("POST "+base+"/api/v1/root/schedule-requests/{request}/decision", s.postRootScheduleDecision)
}

func (s *Server) registerRootInstallationSettingsRoutes(mux *http.ServeMux, base string) {
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/settings",
		s.getRootTargetSettings,
	)
	mux.HandleFunc(
		"PUT "+base+"/api/v1/root/installations/{target}/settings",
		s.putRootInstallationSettingsBatch,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/settings/checkpoints/baseline",
		s.getRootInstallationSettingsBaseline,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/settings/checkpoints/{checkpoint}",
		s.getRootInstallationSettingsCheckpoint,
	)
	mux.HandleFunc(
		"POST "+base+"/api/v1/root/installations/{target}/settings/checkpoints/{checkpoint}/restore",
		s.postRootInstallationSettingsRestore,
	)
}

func (s *Server) registerRootTargetAuditRoute(mux *http.ServeMux, base string) {
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/audit",
		s.getRootTargetAudit,
	)
}

// Announce tells connected browsers which catalog or setting scope changed.
func (s *Server) Announce(targetID, repositoryID string) {
	event := panelEvent{Type: "target.changed", TargetID: targetID}
	if repositoryID != "" {
		event.Type = "repository.changed"
		event.RepositoryID = repositoryID
	}
	s.events.announce(event)
}

// AnnounceCatalog tells browsers that one complete installation snapshot has
// committed. It emits even when the new snapshot is empty, so removing the
// final installation cannot leave an open panel displaying stale targets.
func (s *Server) AnnounceCatalog() {
	s.events.announce(panelEvent{Type: panelEventResync})
}

// AnnounceQueue tells scoped browsers that durable background work changed.
func (s *Server) AnnounceQueue(targetID string) {
	s.events.announce(panelEvent{Type: panelEventQueueChanged, TargetID: targetID})
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The static SvelteKit document supplies the resource policy as a hash-based
		// CSP meta tag so its generated bootstrap can run. frame-ancestors is the
		// one directive browsers ignore in a meta policy, so it remains a header.
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) viewer(r *http.Request) (storage.Account, string, error) {
	account, session, err := s.viewerSession(r)

	return account, session.TokenHash, err
}

// viewerSession is viewer with the session record itself, which the caller that
// can write a response header needs in order to renew it.
func (s *Server) viewerSession(r *http.Request) (storage.Account, storage.Session, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return storage.Account{}, storage.Session{}, storage.ErrNotFound
	}
	hash := tokenHash(cookie.Value)
	session, err := s.store.GetSession(r.Context(), hash, s.now())
	if err != nil {
		return storage.Account{}, storage.Session{}, err
	}
	account, err := s.store.GetAccount(r.Context(), session.AccountID)
	if err != nil {
		return storage.Account{}, storage.Session{}, err
	}
	user, err := s.store.GetPanelUser(r.Context(), session.AccountID)
	if err != nil {
		return storage.Account{}, storage.Session{}, err
	}
	if user.Status != storage.PanelUserActive {
		reason := "Your panel access was revoked"
		if user.BanReason != nil {
			reason = *user.BanReason
		}
		return storage.Account{}, storage.Session{}, storage.SessionRevokedError{
			Code: string(user.Status), Reason: reason,
		}
	}
	session.TokenHash = hash

	return account, session, nil
}

func (s *Server) requireViewer(w http.ResponseWriter, r *http.Request) (storage.Account, bool) {
	account, _, ok := s.requireViewerSession(w, r)

	return account, ok
}

func (s *Server) requireViewerSession(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, string, bool) {
	account, session, err := s.viewerSession(r)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrExpired) {
			s.writeError(w, http.StatusUnauthorized, "unauthenticated", "sign in to use the panel")
		} else if errors.Is(err, storage.ErrRevoked) {
			s.writeError(w, http.StatusUnauthorized, "session_revoked", err.Error())
		} else {
			s.writeInternal(w, err)
		}

		return storage.Account{}, "", false
	}

	/* Every authenticated request passes through here, which is what makes this
	   the place to notice that a session is running out while it is being used.
	   It writes nothing on almost all of them. */
	s.renewSession(w, r, session)

	return account, session.TokenHash, true
}

func (s *Server) requireRoot(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, storage.PanelUser, bool) {
	account, user, _, ok := s.requireRootSession(w, r)

	return account, user, ok
}

func (s *Server) requireRootSession(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, storage.PanelUser, string, bool) {
	account, sessionHash, ok := s.requireViewerSession(w, r)
	if !ok {
		return storage.Account{}, storage.PanelUser{}, "", false
	}
	user, err := s.store.GetPanelUser(r.Context(), account.ID)
	if err != nil {
		s.writeInternal(w, err)
		return storage.Account{}, storage.PanelUser{}, "", false
	}
	if !user.SystemRole.IsRoot() {
		s.writeError(w, http.StatusForbidden, "forbidden", "Root access is required")
		return storage.Account{}, storage.PanelUser{}, "", false
	}

	return account, user, sessionHash, true
}

func (s *Server) requireTarget(
	w http.ResponseWriter,
	r *http.Request,
	write bool,
) (storage.Account, storage.Target, storage.TargetAccess, bool) {
	account, ok := s.requireViewer(w, r)
	if !ok {
		return storage.Account{}, storage.Target{}, storage.TargetAccess{}, false
	}
	targetID := r.PathValue("target")
	access, err := s.store.ResolveTargetAccess(r.Context(), account.ID, targetID, s.now().UTC())
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, "not_found", "installation target not found")
		} else {
			s.writeInternal(w, err)
		}
		return storage.Account{}, storage.Target{}, storage.TargetAccess{}, false
	}
	if !access.Capabilities.Read || write && !access.Capabilities.Write {
		s.writeError(w, http.StatusNotFound, "not_found", "installation target not found")
		return storage.Account{}, storage.Target{}, storage.TargetAccess{}, false
	}
	target, err := s.store.GetTarget(r.Context(), targetID)
	if err != nil {
		s.writeStorageError(w, err)
		return storage.Account{}, storage.Target{}, storage.TargetAccess{}, false
	}

	return account, target, access, true
}

func (s *Server) requireSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") != s.cfg.PublicOrigin {
		s.writeError(w, http.StatusForbidden, "forbidden", "request origin is not allowed")
		return false
	}

	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	return decodeJSONWithin(w, r, target, maxRequestBody)
}

// decodeJSONWithin is the same read against a bound the caller chooses, for the
// endpoints that carry a document rather than a form.
func decodeJSONWithin(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		// A body over the bound is not malformed, and saying it is sends
		// somebody looking for a syntax error that is not there.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large",
				fmt.Sprintf("the request is larger than %d bytes", limit))

			return false
		}

		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must contain one JSON value")
		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{jsonFieldCode: code, jsonFieldMessage: message},
	})
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	writeError(w, status, code, message)
}

func (s *Server) writeInternal(w http.ResponseWriter, _ error) {
	s.writeError(w, http.StatusInternalServerError, "internal", "the panel request failed")
}

func (s *Server) writeStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "not_found", "the requested panel record was not found")
	case errors.Is(err, storage.ErrConflict):
		s.writeError(w, http.StatusConflict, "conflict", "settings changed in another session; reload the latest values")
	default:
		s.writeInternal(w, err)
	}
}
