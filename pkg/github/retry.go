package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	// maxRetries bounds how many extra attempts a transient failure gets.
	maxRetries = 3

	// retryBaseDelay is the first backoff; each further attempt doubles it.
	retryBaseDelay = time.Second

	// maxRetryAfter caps how long GitHub can ask this client to wait. A
	// secondary rate limit can name minutes, and a request path holding a
	// worker that long is worse than failing and letting the delivery layer
	// retry with its own, much longer, schedule.
	maxRetryAfter = 30 * time.Second
)

// retryTransport retries the requests that can succeed on a second attempt.
//
// This sits in the transport rather than around each call because it is the one
// place every request passes through, including the ones go-github's own
// service methods build. The hand-rolled client wrapped retry around its
// request helper instead, which is how the codebase came to hold two retry
// policies that disagreed about 403, 408, 409 and 425.
//
// Retry-After is honoured where the old policy ignored it. GitHub sends it on
// exactly the failures worth waiting for, and guessing an exponential backoff
// against a limit that names its own reset is how a client earns a longer one.
type retryTransport struct {
	base     http.RoundTripper
	attempts int
	sleep    func(*http.Request, time.Duration) error
}

// noRetryKey marks a request that must be answered once and reported as it
// came back.
type noRetryKey struct{}

// withoutRetry returns a context whose requests are sent exactly once.
//
// The readiness probe is the caller this exists for: it asks whether GitHub is
// answering *now*, and a patient answer is the wrong answer. Retrying there
// would keep the pod in service for the whole backoff while it is not.
func withoutRetry(ctx context.Context) context.Context {
	return context.WithValue(ctx, noRetryKey{}, true)
}

func retriesDisabled(ctx context.Context) bool {
	disabled, _ := ctx.Value(noRetryKey{}).(bool)

	return disabled
}

func (t retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if retriesDisabled(req.Context()) {
		return t.base.RoundTrip(req)
	}

	attempts := t.attempts
	if attempts <= 0 {
		attempts = maxRetries
	}

	var lastErr error

	for attempt := range attempts {
		if attempt > 0 {
			// A request whose body cannot be rewound cannot be replayed.
			// go-github builds bodies from a bytes.Reader, so GetBody is set;
			// a caller that hands over an opaque stream gets one attempt.
			if err := rewind(req); err != nil {
				break
			}
		}

		resp, err := t.base.RoundTrip(req)
		if err != nil {
			lastErr = err

			if !t.wait(req, attempt, nil) {
				return nil, err
			}

			continue
		}

		if !retryableStatus(resp) {
			return resp, nil
		}

		if !t.wait(req, attempt, resp) {
			return resp, nil
		}

		// The response is being abandoned. Draining it first is what lets the
		// connection go back to the pool instead of being torn down, which
		// matters most in exactly this case: a retry is about to open another.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return t.base.RoundTrip(req)
}

// wait sleeps before the next attempt and reports whether there is one.
func (t retryTransport) wait(req *http.Request, attempt int, resp *http.Response) bool {
	attempts := t.attempts
	if attempts <= 0 {
		attempts = maxRetries
	}

	if attempt >= attempts-1 {
		return false
	}

	delay := retryBaseDelay << attempt
	if after, ok := retryAfter(resp); ok {
		delay = after
	}

	sleep := t.sleep
	if sleep == nil {
		sleep = sleepContext
	}

	return sleep(req, delay) == nil
}

func rewind(req *http.Request) error {
	if req.Body == nil || req.GetBody == nil {
		if req.Body == nil {
			return nil
		}

		return errors.New("request body cannot be replayed")
	}

	body, err := req.GetBody()
	if err != nil {
		return err
	}

	req.Body = body

	return nil
}

// retryableStatus reports whether repeating this request could plausibly
// produce a different answer.
//
// 403 is here only when GitHub attached a Retry-After, which is how it spells a
// secondary rate limit. A bare 403 is a permission the App was never granted,
// and retrying that just spends the budget faster.
func retryableStatus(resp *http.Response) bool {
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return true
	case resp.StatusCode >= http.StatusInternalServerError:
		return true
	case resp.StatusCode == http.StatusForbidden:
		_, ok := retryAfter(resp)

		return ok
	default:
		return false
	}
}

// retryAfter reads the delay GitHub asked for, in the two spellings it uses.
func retryAfter(resp *http.Response) (time.Duration, bool) {
	if resp == nil {
		return 0, false
	}

	if raw := resp.Header.Get("Retry-After"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
			return capRetryAfter(time.Duration(seconds) * time.Second), true
		}
	}

	// The primary limit names an absolute reset instead of a duration, and only
	// when the remaining budget is actually exhausted. A request that merely
	// failed while the budget is healthy should not wait for the hour boundary.
	if resp.Header.Get("X-RateLimit-Remaining") != "0" {
		return 0, false
	}

	reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return 0, false
	}

	wait := time.Until(time.Unix(reset, 0))
	if wait <= 0 {
		return 0, false
	}

	return capRetryAfter(wait), true
}

func capRetryAfter(delay time.Duration) time.Duration {
	if delay > maxRetryAfter {
		return maxRetryAfter
	}

	return delay
}

// sleepContext waits, but gives up the moment the caller does. A long-running
// service cancels on shutdown, and a sleep that ignored it would outlive the
// cancellation by the whole backoff.
func sleepContext(req *http.Request, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-req.Context().Done():
		return req.Context().Err()
	case <-timer.C:
		return nil
	}
}
