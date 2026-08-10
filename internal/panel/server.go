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

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const maxRequestBody = 64 << 10

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

// Dependencies are the service capabilities used by panel handlers.
type Dependencies struct {
	Store   storage.Store
	Catalog catalogSyncer
	Users   userResolver
	SignIn  signInProvider
	Random  io.Reader
	Now     func() time.Time
	Runtime RuntimeController
}

// Server owns the panel routes and their authenticated runtime state.
type Server struct {
	cfg        Config
	store      storage.Store
	catalog    catalogSyncer
	users      userResolver
	signIn     signInProvider
	random     io.Reader
	now        func() time.Time
	startedAt  time.Time
	assets     *assetBundle
	events     *eventHub
	runtimeMu  sync.RWMutex
	runtime    RuntimeValues
	controller RuntimeController
}

// New creates a production panel server.
func New(cfg Config, deps Dependencies) (*Server, error) {
	validated, err := cfg.validated()
	if err != nil {
		return nil, err
	}
	if deps.Store == nil || deps.Catalog == nil || deps.Users == nil {
		return nil, fmt.Errorf(
			"%w: storage, catalog sync, and user lookup are required",
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
	if deps.Runtime != nil {
		deps.Runtime.ApplyRuntimeSettings(runtime)
	}

	return &Server{
		cfg:        validated,
		store:      deps.Store,
		catalog:    deps.Catalog,
		users:      deps.Users,
		signIn:     deps.SignIn,
		random:     deps.Random,
		now:        deps.Now,
		startedAt:  deps.Now().UTC(),
		assets:     assets,
		events:     newEventHub(),
		runtime:    runtime,
		controller: deps.Runtime,
	}, nil
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
	mux.HandleFunc("PUT "+base+"/api/v1/targets/{target}/settings", s.putTargetSettings)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/users", s.getTargetUsers)
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
		"PUT "+base+"/api/v1/targets/{target}/repositories/{repository}/settings",
		s.putRepositorySettings,
	)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/audit", s.getAudit)
	mux.HandleFunc("GET "+base+"/api/v1/targets/{target}/failures", s.getFailures)
	mux.HandleFunc("GET "+base+"/api/v1/events", s.streamEvents)

	if base != "" {
		mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, base+"/", http.StatusPermanentRedirect)
		})
	}
	mux.HandleFunc("GET "+base+"/", s.serveAsset)

	return s.secureHeaders(mux)
}

func (s *Server) registerRootRoutes(mux *http.ServeMux, base string) {
	mux.HandleFunc("GET "+base+"/api/v1/root/installations", s.getRootInstallations)
	mux.HandleFunc("GET "+base+"/api/v1/root/overview", s.getRootOverview)
	mux.HandleFunc("GET "+base+"/api/v1/root/settings", s.getRootRuntimeSettings)
	mux.HandleFunc("PUT "+base+"/api/v1/root/settings", s.putRootRuntimeSettings)
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
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/settings",
		s.getRootTargetSettings,
	)
	mux.HandleFunc(
		"PUT "+base+"/api/v1/root/installations/{target}/settings",
		s.putRootTargetSettings,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/repositories",
		s.getRootRepositories,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/repositories/{repository}",
		s.getRootRepository,
	)
	mux.HandleFunc(
		"PUT "+base+"/api/v1/root/installations/{target}/repositories/{repository}/settings",
		s.putRootRepositorySettings,
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
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/audit",
		s.getRootTargetAudit,
	)
	mux.HandleFunc(
		"GET "+base+"/api/v1/root/installations/{target}/failures",
		s.getRootTargetFailures,
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

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' https:; style-src 'self'; script-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self' https://github.com")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) viewer(r *http.Request) (storage.Account, string, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return storage.Account{}, "", storage.ErrNotFound
	}
	hash := tokenHash(cookie.Value)
	session, err := s.store.GetSession(r.Context(), hash, s.now())
	if err != nil {
		return storage.Account{}, "", err
	}
	account, err := s.store.GetAccount(r.Context(), session.AccountID)
	if err != nil {
		return storage.Account{}, "", err
	}
	user, err := s.store.GetPanelUser(r.Context(), session.AccountID)
	if err != nil {
		return storage.Account{}, "", err
	}
	if user.Status != storage.PanelUserActive {
		reason := "Your panel access was revoked"
		if user.BanReason != nil {
			reason = *user.BanReason
		}
		return storage.Account{}, "", storage.SessionRevokedError{
			Code: string(user.Status), Reason: reason,
		}
	}

	return account, hash, nil
}

func (s *Server) requireViewer(w http.ResponseWriter, r *http.Request) (storage.Account, bool) {
	account, _, ok := s.requireViewerSession(w, r)

	return account, ok
}

func (s *Server) requireViewerSession(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, string, bool) {
	account, sessionHash, err := s.viewer(r)
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

	return account, sessionHash, true
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
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
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
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
