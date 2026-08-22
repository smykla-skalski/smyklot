package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// configServer answers the two questions reading a repository's configuration
// asks - what does the root look like, and what is at this path - and counts
// both, so a spec can assert what a tick actually costs.
type configServer struct {
	mu sync.Mutex

	// rootSHA is the .github tree's SHA. Changing it is how a spec says
	// something a configuration file could live in has moved.
	rootSHA string

	// present is the one path that exists, or empty for a repository with no
	// configuration file.
	present string

	fingerprints int
	probes       []string
}

func (s *configServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.HasSuffix(r.URL.Path, "/contents") {
		s.fingerprints++
		_, _ = w.Write([]byte(`[{"name":".github","sha":"` + s.rootSHA + `"}]`))

		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/repos/acme/web/contents/")
	s.probes = append(s.probes, path)

	if s.present != "" && path == s.present {
		_, _ = w.Write([]byte(githubtest.ContentsResponse("quiet_success = true\n")))

		return
	}

	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"message":"Not Found"}`))
}

func (s *configServer) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.fingerprints, len(s.probes)
}

func newConfigCacheHarness(t *testing.T, stub *configServer) (*repoCache[repositoryConfigFile], *github.Client) {
	t.Helper()

	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)

	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// A zero TTL means every read is a miss, which is what the service does:
	// repoConfigTTL is deliberately shorter than the sweep interval, so the
	// entry has always expired by the next tick. What must not happen is that
	// expiring costs a re-probe.
	return newRepoCache(0, fetchRepositoryConfig), client
}

// The property the whole design rests on: a tick on a repository whose
// configuration cannot have changed costs one request, not one per candidate
// path. Nothing exercised this before, which is how a deadlock in the same code
// went unnoticed.
func TestRepositoryConfigTickCostsOneRequestWhenNothingMoved(t *testing.T) {
	t.Parallel()

	stub := &configServer{rootSHA: "gh-1", present: ".github/smyklot.yaml"}
	cache, client := newConfigCacheHarness(t, stub)

	if _, err := cache.Get(t.Context(), client, "acme", "web"); err != nil {
		t.Fatalf("first read: %v", err)
	}

	_, probesAfterFirst := stub.counts()

	for range 5 {
		if _, err := cache.Get(t.Context(), client, "acme", "web"); err != nil {
			t.Fatalf("later read: %v", err)
		}
	}

	fingerprints, probes := stub.counts()

	if probes != probesAfterFirst {
		t.Errorf("later ticks probed %d more paths, want none", probes-probesAfterFirst)
	}

	if fingerprints != 6 {
		t.Errorf("6 reads asked for the fingerprint %d times, want 6", fingerprints)
	}
}

// And it must still notice a change, or a repository that edits its
// configuration is answered from a cache that can never expire.
func TestRepositoryConfigIsRereadWhenARootMoves(t *testing.T) {
	t.Parallel()

	stub := &configServer{rootSHA: "gh-1", present: ".github/smyklot.yaml"}
	cache, client := newConfigCacheHarness(t, stub)

	first, err := cache.Get(t.Context(), client, "acme", "web")
	if err != nil {
		t.Fatalf("first read: %v", err)
	}

	if first.path != ".github/smyklot.yaml" {
		t.Fatalf("first read found %q", first.path)
	}

	stub.mu.Lock()
	stub.rootSHA = "gh-2"
	stub.present = ".github/.smyklot.toml"
	stub.mu.Unlock()

	_, probesBefore := stub.counts()

	second, err := cache.Get(t.Context(), client, "acme", "web")
	if err != nil {
		t.Fatalf("second read: %v", err)
	}

	if second.path != ".github/.smyklot.toml" {
		t.Errorf("after the root moved, read %q, want the TOML file", second.path)
	}

	if _, probesAfter := stub.counts(); probesAfter == probesBefore {
		t.Error("the root moved and nothing was re-probed")
	}
}

// A repository with no configuration file is the common case, and the one whose
// cost matters most. It must be cached exactly like a file that was found.
func TestRepositoryWithNoConfigIsCachedToo(t *testing.T) {
	t.Parallel()

	stub := &configServer{rootSHA: "gh-1"}
	cache, client := newConfigCacheHarness(t, stub)

	for range 4 {
		if _, err := cache.Get(t.Context(), client, "acme", "web"); err != nil {
			t.Fatalf("read: %v", err)
		}
	}

	fingerprints, probes := stub.counts()

	if probes != len(config.RepoConfigPaths) {
		t.Errorf("probed %d paths over four ticks, want %d - one sweep of the candidates",
			probes, len(config.RepoConfigPaths))
	}

	if fingerprints != 4 {
		t.Errorf("four ticks asked for the fingerprint %d times, want 4", fingerprints)
	}
}
