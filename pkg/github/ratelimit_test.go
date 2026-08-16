package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// secondaryRateLimitBody is what GitHub answers a request that tripped abuse
// detection.
//
// documentation_url is the load-bearing field, not the message: go-github
// classifies a 403 as a secondary rate limit by that suffix alone. A fixture
// carrying only the message and a Retry-After header - which is what this spec
// used to send - comes back as an ordinary error response, so nothing is
// remembered, nothing is capped, and the spec proves nothing while still
// costing twenty seconds of retry backoff to say so.
const secondaryRateLimitBody = `{"message":"You have exceeded a secondary rate limit",` +
	`"documentation_url":"https://docs.github.com/rest/using-the-rest-api/` +
	`rate-limits-for-the-rest-api#secondary-rate-limits"}`

// TestSecondaryRateLimitWindowIsBounded proves go-github's memory of a
// secondary rate limit expires when this package says so rather than when
// GitHub asked.
//
// go-github remembers a secondary rate limit and then answers later calls on
// the same client itself, without touching the network - which also means
// without passing through this package's retry transport, so its own cap on how
// long to wait never applies.
//
// A client is not per-call: the sweep mints one and reuses it for every
// repository in an installation. Left unbounded, one request tripping abuse
// detection fails every repository left in that sweep for as long as GitHub
// named, which its documentation says can be minutes.
//
// Deliberately the slowest test in the package: proving the window is bounded
// means waiting for it to pass, and a bound nobody waits out is a bound nobody
// has checked. It waits out that window and nothing else, which is why it is
// internal - maxRetryAfter caps the backoff between attempts too, so through the
// exported client each call here would first spend three capped backoffs
// proving what retry_internal_test.go already proves without waiting.
func TestSecondaryRateLimitWindowIsBounded(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)

		// An hour is well past anything this service should honour inline.
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(secondaryRateLimitBody))
	}))
	defer server.Close()

	client, err := NewClient("test-token", server.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(withoutRetry(t.Context()), time.Minute)
	defer cancel()

	label := func() error {
		return client.AddLabel(ctx, "acme", "web", 42, "kind/bug")
	}

	// The first call trips the limit.
	if err := label(); err == nil {
		t.Fatal("AddLabel succeeded against a server that only ever refuses")
	}

	tripped := calls.Load()
	if tripped != 1 {
		t.Fatalf("requests that reached the server = %d, want exactly one", tripped)
	}

	// The precondition, asserted rather than assumed. A client that is not
	// holding the limit in memory answers the next call from the network, and
	// then the wait below measures nothing at all - which is how this spec spent
	// its whole life passing without a cap under test.
	start := time.Now()
	if err := label(); err == nil {
		t.Fatal("AddLabel succeeded while the client was holding a secondary rate limit")
	}

	if calls.Load() != tripped {
		t.Fatal("the client sent the next request rather than refusing it from memory")
	}

	// It must come back to the network once the cap has elapsed. Without the
	// cap, go-github answers from memory for the whole hour GitHub named and
	// the request count never moves again.
	for calls.Load() == tripped {
		if waited := time.Since(start); waited > maxRetryAfter+5*time.Second {
			t.Fatalf(
				"still refusing from memory after %s, cap is %s",
				waited.Round(time.Millisecond), maxRetryAfter,
			)
		}

		_ = label()
		time.Sleep(25 * time.Millisecond)
	}
}
