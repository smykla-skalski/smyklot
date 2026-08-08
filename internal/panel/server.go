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
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

const maxRequestBody = 64 << 10

type catalogSyncer interface {
	SyncCatalog(context.Context) ([]string, error)
}

// Dependencies are the service capabilities used by panel handlers.
type Dependencies struct {
	Store   storage.Store
	Catalog catalogSyncer
	SignIn  signInProvider
	Random  io.Reader
	Now     func() time.Time
}

// Server owns the panel routes and their authenticated runtime state.
type Server struct {
	cfg     Config
	store   storage.Store
	catalog catalogSyncer
	signIn  signInProvider
	random  io.Reader
	now     func() time.Time
	assets  *assetBundle
	events  *eventHub
}

// New creates a production panel server.
func New(cfg Config, deps Dependencies) (*Server, error) {
	validated, err := cfg.validated()
	if err != nil {
		return nil, err
	}
	if deps.Store == nil || deps.Catalog == nil {
		return nil, fmt.Errorf("%w: storage and catalog sync are required", errInvalidConfig)
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

	return &Server{
		cfg:     validated,
		store:   deps.Store,
		catalog: deps.Catalog,
		signIn:  deps.SignIn,
		random:  deps.Random,
		now:     deps.Now,
		assets:  assets,
		events:  newEventHub(),
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
	mux.HandleFunc("PUT "+base+"/api/v1/targets/{target}/settings", s.putTargetSettings)
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

// Announce tells connected browsers which catalog or setting scope changed.
func (s *Server) Announce(targetID, repositoryID string) {
	event := panelEvent{Type: "target", TargetID: targetID}
	if repositoryID != "" {
		event.Type = "repository"
		event.RepositoryID = repositoryID
	}
	s.events.announce(event)
}

// AnnounceCatalog tells browsers that one complete installation snapshot has
// committed. It emits even when the new snapshot is empty, so removing the
// final installation cannot leave an open panel displaying stale targets.
func (s *Server) AnnounceCatalog() {
	s.events.announce(panelEvent{Type: "resync"})
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

	return account, hash, err
}

func (s *Server) requireViewer(w http.ResponseWriter, r *http.Request) (storage.Account, bool) {
	account, _, err := s.viewer(r)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrExpired) {
			s.writeError(w, http.StatusUnauthorized, "unauthenticated", "sign in to use the panel")
		} else {
			s.writeInternal(w, err)
		}

		return storage.Account{}, false
	}

	return account, true
}

func (s *Server) requireTarget(
	w http.ResponseWriter,
	r *http.Request,
) (storage.Account, storage.Target, bool) {
	account, ok := s.requireViewer(w, r)
	if !ok {
		return storage.Account{}, storage.Target{}, false
	}
	targetID := r.PathValue("target")
	allowed, err := s.store.CanAccessTarget(r.Context(), account.ID, targetID)
	if err != nil {
		s.writeInternal(w, err)
		return storage.Account{}, storage.Target{}, false
	}
	if !allowed {
		s.writeError(w, http.StatusNotFound, "not_found", "installation target not found")
		return storage.Account{}, storage.Target{}, false
	}
	target, err := s.store.GetTarget(r.Context(), targetID)
	if err != nil {
		s.writeStorageError(w, err)
		return storage.Account{}, storage.Target{}, false
	}

	return account, target, true
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
