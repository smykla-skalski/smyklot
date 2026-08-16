package github

import (
	"context"
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
	//
	// It bounds two different waits. Here it caps the backoff between attempts.
	// It is also handed to go-github, which otherwise remembers a secondary
	// limit for as long as GitHub named and answers every later call on the
	// same client from memory - see newGoGitHub.
	maxRetryAfter = 5 * time.Second

	// attemptTimeout bounds one attempt, not the call. See retryTransport.attempt.
	attemptTimeout = 30 * time.Second
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
	base  http.RoundTripper
	sleep func(*http.Request, time.Duration) error
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
	// Still through attempt, not straight to base. These two paths send once
	// rather than never, and the deadline is not part of retrying - routing
	// them around it left the readiness probe and any non-replayable request
	// with no timeout at all, since the client no longer carries one.
	if retriesDisabled(req.Context()) || !replayable(req) {
		return t.attempt(req)
	}

	last := maxRetries - 1

	for attempt := 0; ; attempt++ {
		resp, err := t.attempt(req)

		// Every path out of this loop is a return, so the caller always gets
		// either the last response or the last error - never a fresh request
		// made after the budget was spent.
		switch {
		case err != nil && attempt >= last:
			return nil, err

		case err != nil:
			if t.wait(req, attempt, nil) != nil || rewind(req) != nil {
				return nil, err
			}

		case !retryableStatus(resp) || attempt >= last:
			return resp, nil

		default:
			if t.wait(req, attempt, resp) != nil || rewind(req) != nil {
				return resp, nil
			}

			// Only now is the response certainly being abandoned. Draining it
			// is what lets the connection go back to the pool instead of being
			// torn down, which matters most here: a retry is about to want one.
			drain(resp)
		}
	}
}

// attempt sends the request once, under its own deadline.
//
// The deadline is per attempt rather than per call. An http.Client.Timeout
// would cover the whole RoundTrip, and since retry now lives inside the
// transport that would mean three attempts and their backoffs sharing one
// budget - so a first attempt that hung would leave the retries no time to run
// and the backoff would be spent waiting for a request that could never be
// made. The hand-rolled client gave each attempt a fresh timeout by sitting
// above the client rather than inside it; this keeps that.
//
// The cancel cannot fire when this returns, because the body is read by the
// caller afterwards. It is tied to the body instead, which is the same thing
// http.Client.Timeout does internally.
func (t retryTransport) attempt(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), attemptTimeout)

	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()

		return nil, err
	}

	resp.Body = cancelOnClose{ReadCloser: resp.Body, cancel: cancel}

	return resp, nil
}

// cancelOnClose releases an attempt's context once its body is done with.
type cancelOnClose struct {
	io.ReadCloser

	cancel context.CancelFunc
}

func (b cancelOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()

	return err
}

// wait sleeps for as long as the next attempt should be delayed.
func (t retryTransport) wait(req *http.Request, attempt int, resp *http.Response) error {
	delay := retryBaseDelay << attempt
	if after, ok := retryAfter(resp); ok {
		delay = after
	}

	// Capped here, once, rather than inside retryAfter. There it clamped only
	// the header-derived branch and ran for nothing when retryableStatus asked
	// merely whether a delay existed - leaving the exponential branch the one
	// path that could exceed the cap if either constant above ever changed.
	delay = min(delay, maxRetryAfter)

	sleep := t.sleep
	if sleep == nil {
		sleep = sleepContext
	}

	return sleep(req, delay)
}

// replayable reports whether this request can be sent a second time.
//
// go-github builds bodies from a bytes.Reader, so GetBody is set and every
// request it makes can be replayed. A caller handing over an opaque stream gets
// one attempt, decided here rather than after a failure - discovering it later
// would mean having already slept, and having drained the answer that should
// have been returned.
func replayable(req *http.Request) bool {
	return req.Body == nil || req.GetBody != nil
}

func rewind(req *http.Request) error {
	if req.Body == nil || req.GetBody == nil {
		return nil
	}

	body, err := req.GetBody()
	if err != nil {
		return err
	}

	req.Body = body

	return nil
}

// drain reads what is left of an abandoned response so its connection can be
// reused, bounded so a large error page cannot make this expensive.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
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
			return time.Duration(seconds) * time.Second, true
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

	return wait, true
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
