// Package githubapp mints GitHub App tokens.
//
// The Action knows its installation before it starts, so it needs one token and
// exits. A service learns the installation from each delivery and may serve
// hundreds, so it needs a token per installation and needs them to outlive the
// request that first asked for one.
package githubapp

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/jferrl/go-githubauth"
	"golang.org/x/oauth2"
)

// DefaultMintTimeout bounds one call to GitHub's installation token endpoint.
//
// go-githubauth mints with context.Background() and an http.Client that sets
// only dial and TLS timeouts, so a connection that opens and then goes quiet
// blocks forever. In a service that means one worker lost from the pool for
// good, and a shutdown that waits out its whole drain timeout. Matches the
// timeout the API client uses.
const DefaultMintTimeout = 30 * time.Second

// Sentinel errors for App authentication.
var (
	// ErrNoAppID is returned when neither a client ID nor an app ID is set
	ErrNoAppID = errors.New("no GitHub App client ID or app ID configured")

	// ErrNoPrivateKey is returned when the App private key is missing
	ErrNoPrivateKey = errors.New("no GitHub App private key configured")
)

// TokenStore mints and caches tokens for a single GitHub App.
//
// go-githubauth's sources already refresh a token shortly before it expires, so
// this holds on to one source per installation rather than re-deriving it - and
// with it, the parsed private key - on every delivery.
//
// Safe for concurrent use.
type TokenStore struct {
	appSource   oauth2.TokenSource
	baseURL     string
	mintTimeout time.Duration

	mu      sync.Mutex
	sources map[int64]oauth2.TokenSource
}

// NewTokenStore creates a store for the App identified by clientID.
//
// clientID accepts either the App's client ID (which GitHub recommends) or its
// numeric app ID. baseURL is optional and points token minting at a GitHub
// Enterprise instance; empty uses public GitHub. A non-positive mintTimeout
// falls back to DefaultMintTimeout.
func NewTokenStore(
	clientID string,
	privateKey []byte,
	baseURL string,
	mintTimeout time.Duration,
) (*TokenStore, error) {
	if clientID == "" {
		return nil, ErrNoAppID
	}

	if len(privateKey) == 0 {
		return nil, ErrNoPrivateKey
	}

	if mintTimeout <= 0 {
		mintTimeout = DefaultMintTimeout
	}

	appSource, err := githubauth.NewApplicationTokenSource(clientID, privateKey)
	if err != nil {
		return nil, err
	}

	return &TokenStore{
		appSource:   copyingTokenSource{src: appSource},
		baseURL:     baseURL,
		mintTimeout: mintTimeout,
		sources:     make(map[int64]oauth2.TokenSource),
	}, nil
}

// copyingTokenSource hands every caller its own copy of the token underneath.
//
// go-githubauth caches one *oauth2.Token and returns that same pointer to every
// caller, while oauth2.ReuseTokenSource writes a field on whatever token it is
// handed. Sharing one app token source across installation sources - which is
// the point of this store - therefore puts several of them on one struct. They
// all write the same value, so nothing breaks, but it is a data race and the
// suite runs under -race. Copying at the boundary ends it.
type copyingTokenSource struct {
	src oauth2.TokenSource
}

func (c copyingTokenSource) Token() (*oauth2.Token, error) {
	token, err := c.src.Token()
	if err != nil {
		return nil, err
	}

	dup := *token

	return &dup, nil
}

// AppToken returns a JWT authenticating as the App itself.
//
// Use it only for app-level endpoints, with github.NewAppClient. Every
// repository operation needs InstallationToken instead.
func (s *TokenStore) AppToken() (string, error) {
	token, err := s.appSource.Token()
	if err != nil {
		return "", err
	}

	return token.AccessToken, nil
}

// InstallationToken returns a token acting as the given installation.
func (s *TokenStore) InstallationToken(installationID int64) (string, error) {
	token, err := s.source(installationID).Token()
	if err != nil {
		return "", err
	}

	return token.AccessToken, nil
}

// source returns the cached token source for an installation, creating it on
// first use.
func (s *TokenStore) source(installationID int64) oauth2.TokenSource {
	s.mu.Lock()
	defer s.mu.Unlock()

	if src, ok := s.sources[installationID]; ok {
		return src
	}

	opts := []githubauth.InstallationTokenSourceOpt{
		githubauth.WithHTTPClient(&http.Client{Timeout: s.mintTimeout}),
	}

	if s.baseURL != "" {
		opts = append(opts, githubauth.WithBaseURL(s.baseURL))
	}

	src := githubauth.NewInstallationTokenSource(installationID, s.appSource, opts...)
	s.sources[installationID] = src

	return src
}
