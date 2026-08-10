package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
)

// githubStub is a stand-in GitHub API for the service specs.
//
// It answers the handful of endpoints a command touches and records every call,
// so a spec can assert on what the service did rather than on what it logged.
type githubStub struct {
	// codeowners is the repository's CODEOWNERS, or empty for a repository
	// without one
	codeowners string

	// repoConfig is the repository's .github/smyklot.yaml, or empty for a
	// repository without one
	repoConfig string

	// prAuthor is who opened the pull request, which decides whether a command
	// counts as self-approval
	prAuthor string

	// installations is what GET /app/installations reports, and repos what
	// each installation can reach. Both are empty unless a spec sweeps
	installations string
	repos         string
	members       string
	membersStatus int

	// openPRs is what a repository's pull request list reports
	openPRs string

	// brokenRepo names a repository every request for fails, standing in for
	// one the App has lost access to
	brokenRepo string

	// probeStatus is what GET /app answers, which is the one call the readiness
	// probe makes. Set it before the service starts, never while it is running
	probeStatus int

	installationsStarted chan struct{}
	installationsRelease chan struct{}
	installationsBlock   sync.Once

	mu    sync.Mutex
	calls []string
}

func newGitHubStub() *githubStub {
	return &githubStub{
		codeowners:    "* @someone\n",
		prAuthor:      "author",
		installations: `[]`,
		repos:         `{"total_count": 0, "repositories": []}`,
		members:       `[]`,
		membersStatus: http.StatusOK,
		openPRs:       `[]`,
		probeStatus:   http.StatusOK,
	}
}

func (s *githubStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	s.mu.Unlock()

	switch {
	case s.brokenRepo != "" && strings.Contains(r.URL.Path, "/"+s.brokenRepo+"/"):
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message": "Resource not accessible by integration"}`))

	case r.URL.Path == "/app/installations":
		s.mu.Lock()
		installations := s.installations
		s.mu.Unlock()
		if s.installationsStarted != nil {
			s.installationsBlock.Do(func() {
				close(s.installationsStarted)
				<-s.installationsRelease
			})
		}
		_, _ = io.WriteString(w, installations)

	// The readiness probe's endpoint. It must be matched after
	// /app/installations, which would otherwise never be reached
	case r.URL.Path == "/app":
		if s.probeStatus != http.StatusOK {
			w.WriteHeader(s.probeStatus)
			_, _ = w.Write([]byte(`{"message": "unavailable"}`))

			return
		}

		_, _ = w.Write([]byte(`{"id": 1197525, "slug": "smyklot"}`))

	// GitHub answers this 401 for an App JWT, which is what the probe carries.
	// The stub answering it too would hide a probe pointed back at it
	case r.URL.Path == "/rate_limit":
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))

	case r.URL.Path == "/installation/repositories":
		_, _ = io.WriteString(w, s.repos)

	case strings.HasPrefix(r.URL.Path, "/orgs/") && strings.HasSuffix(r.URL.Path, "/members"):
		if s.membersStatus != http.StatusOK {
			w.WriteHeader(s.membersStatus)
		}
		_, _ = io.WriteString(w, s.members)

	case strings.HasSuffix(r.URL.Path, "/pulls"):
		_, _ = io.WriteString(w, s.openPRs)

	case strings.HasSuffix(r.URL.Path, "/labels"):
		_, _ = w.Write([]byte(`[]`))

	case strings.HasSuffix(r.URL.Path, "/access_tokens"):
		expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"token": "installation-token", "expires_at": %q}`, expiry)

	case strings.HasSuffix(r.URL.Path, "/contents/.github/CODEOWNERS"):
		s.writeFile(w, s.codeowners)

	case strings.Contains(r.URL.Path, "/contents/.github/smyklot."):
		if strings.HasSuffix(r.URL.Path, ".yaml") {
			s.writeFile(w, s.currentRepoConfig())

			return
		}

		s.writeFile(w, "")

	case strings.HasSuffix(r.URL.Path, "/reviews"):
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 1, "state": "APPROVED"}`))

	case strings.HasSuffix(r.URL.Path, "/reactions"):
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 1}`))

	case strings.HasSuffix(r.URL.Path, "/comments"):
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 1}`))

	case strings.Contains(r.URL.Path, "/pulls/"):
		_, _ = fmt.Fprintf(w, `{
			"number": 42,
			"state": "open",
			"mergeable": true,
			"mergeable_state": "clean",
			"title": "a change",
			"user": {"login": %q},
			"base": {"ref": "main"}
		}`, s.prAuthor)

	default:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}
}

func (s *githubStub) setInstallations(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.installations = value
}

// setRepoConfig changes the repository's file while the service is running, the
// way a commit to the default branch does
func (s *githubStub) setRepoConfig(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.repoConfig = content
}

// currentRepoConfig reads the file under the lock, because a spec can change it
// while a worker is serving a delivery
func (s *githubStub) currentRepoConfig() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repoConfig
}

// writeFile answers the contents API, treating empty content as a missing file
func (s *githubStub) writeFile(w http.ResponseWriter, content string) {
	if content == "" {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Not Found"}`))

		return
	}

	_, _ = io.WriteString(w, githubtest.ContentsResponse(content))
}

// countCalls reports how many recorded calls match method and path suffix
func (s *githubStub) countCalls(method, pathSuffix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0

	for _, call := range s.calls {
		if strings.HasPrefix(call, method+" ") && strings.HasSuffix(call, pathSuffix) {
			count++
		}
	}

	return count
}

// total reports how many calls the service made in all
func (s *githubStub) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.calls)
}
