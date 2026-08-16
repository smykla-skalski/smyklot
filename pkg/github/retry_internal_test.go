package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestRetry builds a transport that records what it waited for instead of
// actually waiting, so a backoff spec costs microseconds rather than seconds.
func newTestRetry(base http.RoundTripper, waits *[]time.Duration) retryTransport {
	return retryTransport{
		base: base,
		sleep: func(_ *http.Request, delay time.Duration) error {
			*waits = append(*waits, delay)

			return nil
		},
	}
}

func TestRetryTransportRetriesServerErrors(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusBadGateway)

			return
		}

		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	// Exponential, doubling each time.
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 2*time.Second {
		t.Fatalf("waits = %v, want [1s 2s]", waits)
	}
}

func TestRetryTransportGivesUpAndReturnsTheLastResponse(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	// The caller gets the real answer rather than an invented error.
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}

	if attempts != maxRetries {
		t.Fatalf("attempts = %d, want %d", attempts, maxRetries)
	}
}

func TestRetryTransportDoesNotRetryClientErrors(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

// A bare 403 is a permission the App was never granted. Retrying it only spends
// the rate-limit budget faster.
func TestRetryTransportDoesNotRetryBareForbidden(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

// A 403 carrying Retry-After is how GitHub spells a secondary rate limit, and
// the wait it names beats any backoff this client could guess.
func TestRetryTransportHonoursRetryAfter(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusForbidden)

			return
		}

		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if len(waits) != 1 || waits[0] != 3*time.Second {
		t.Fatalf("waits = %v, want [3s]", waits)
	}
}

func TestRetryTransportCapsRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	for _, wait := range waits {
		if wait > maxRetryAfter {
			t.Fatalf("wait = %v, want no more than %v", wait, maxRetryAfter)
		}
	}
}

// The primary limit names an absolute reset rather than a duration, and only
// matters once the budget is actually spent.
func TestRetryTransportReadsExhaustedPrimaryLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(5*time.Second).Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if len(waits) == 0 {
		t.Fatal("an exhausted primary limit should have been waited on")
	}
}

func TestRetryTransportIgnoresHealthyPrimaryLimit(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 - the budget was healthy", attempts)
	}
}

// A replayed request has to send its body again, or the second attempt writes
// an empty one and GitHub rejects it for a reason that has nothing to do with
// the failure being retried.
func TestRetryTransportReplaysTheRequestBody(t *testing.T) {
	var bodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))

		if len(bodies) < 2 {
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, server.URL, strings.NewReader(`{"labels":["kind/bug"]}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if len(bodies) != 2 {
		t.Fatalf("attempts = %d, want 2", len(bodies))
	}

	if bodies[0] != bodies[1] {
		t.Fatalf("second attempt sent %q, want %q", bodies[1], bodies[0])
	}
}

// A body that cannot be rewound means one attempt, and that has to be decided
// before anything is sent. Deciding it after a failure would mean the sleep had
// already happened and the answer worth returning had already been drained -
// and an earlier draft then issued a *fourth* request with a spent body.
func TestRetryTransportSendsUnreplayableBodyExactlyOnce(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	// An opaque stream: http.NewRequestWithContext cannot derive GetBody from
	// a reader it does not recognise.
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, server.URL,
		struct{ io.Reader }{strings.NewReader(`{"labels":["kind/bug"]}`)},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 - the body cannot be replayed", attempts)
	}

	if len(waits) != 0 {
		t.Fatalf("waits = %v, want none - nothing was going to be retried", waits)
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 - the real answer, not a later one", resp.StatusCode)
	}
}

func TestRetryTransportSkipsRetryWhenDisabled(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	req, err := http.NewRequestWithContext(
		withoutRetry(context.Background()), http.MethodGet, server.URL, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 - a readiness probe wants the current answer", attempts)
	}
}

// Each attempt carries its own deadline, so a first attempt that burns most of
// one does not leave the retries with nothing. A single shared budget - which
// is what an http.Client.Timeout would give, now that retry lives inside the
// transport - would let one slow response spend the whole allowance.
func TestRetryTransportGivesEachAttemptItsOwnDeadline(t *testing.T) {
	var deadlines []time.Time

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok {
			t.Error("attempt had no deadline")
		}

		deadlines = append(deadlines, deadline)

		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{},
		}, nil
	})

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(base, &waits)}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if len(deadlines) != maxRetries {
		t.Fatalf("attempts = %d, want %d", len(deadlines), maxRetries)
	}

	// A shared budget would make every deadline the same instant.
	for index := 1; index < len(deadlines); index++ {
		if !deadlines[index].After(deadlines[index-1]) {
			t.Fatalf("attempt %d reused the previous deadline - the budget is shared", index)
		}
	}
}

// The body outlives RoundTrip, so its attempt's context must too. Cancelling on
// return would make every response body fail to read.
func TestRetryTransportBodyOutlivesItsAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	var waits []time.Duration

	client := &http.Client{Transport: newTestRetry(http.DefaultTransport, &waits)}

	resp, err := client.Get(server.URL) //nolint:noctx // the transport under test reads the request context
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the body failed: %v", err)
	}

	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q", body)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRetryTransportStopsWhenTheCallerGivesUp(t *testing.T) {
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	transport := retryTransport{
		base: http.DefaultTransport,
		sleep: func(req *http.Request, _ time.Duration) error {
			cancel()

			return req.Context().Err()
		},
	}

	client := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 - the context was cancelled during the backoff", attempts)
	}
}
