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

	// openPRs is what a repository's pull request list reports
	openPRs string

	// brokenRepo names a repository every request for fails, standing in for
	// one the App has lost access to
	brokenRepo string

	mu    sync.Mutex
	calls []string
}

func newGitHubStub() *githubStub {
	return &githubStub{
		codeowners:    "* @someone\n",
		prAuthor:      "author",
		installations: `[]`,
		repos:         `{"total_count": 0, "repositories": []}`,
		openPRs:       `[]`,
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
		_, _ = io.WriteString(w, s.installations)

	case r.URL.Path == "/installation/repositories":
		_, _ = io.WriteString(w, s.repos)

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
			s.writeFile(w, s.repoConfig)

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
