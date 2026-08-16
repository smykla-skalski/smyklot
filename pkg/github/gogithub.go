package github

import (
	"context"
	"errors"
	"net/http"
	"strings"

	gogithub "github.com/google/go-github/v90/github"
)

// newGoGitHub builds the typed client this package speaks GitHub through.
//
// WithURLs, not WithEnterpriseURLs. The latter appends /api/v3/ to whatever it
// is given, which is right for a real Enterprise host and wrong for everything
// this package is pointed at: SMYKLOT_GITHUB_API_URL names a proxy or mirror
// that already serves the API at its root, and every spec in this package hands
// it an httptest server that serves /repos/... directly. WithURLs takes the
// base verbatim.
//
// go-github resolves each endpoint as a relative reference against the base, so
// the base needs a trailing slash or the last path segment is discarded.
// newClient has just stripped one, deliberately, because the hand-rolled
// request path concatenates and would otherwise double it.
func newGoGitHub(httpClient *http.Client, baseURL string) (*gogithub.Client, error) {
	base := strings.TrimSuffix(baseURL, "/") + "/"

	return gogithub.NewClient(
		gogithub.WithHTTPClient(httpClient),
		gogithub.WithUserAgent(userAgent),
		gogithub.WithURLs(&base, &base),

		// Bound go-github's own secondary-rate-limit memory.
		//
		// On a 403 naming a Retry-After, go-github records the reset time and
		// then short-circuits every later call on the same client until it
		// passes - returning a synthetic 403 without touching the network, and
		// therefore without passing through retryTransport at all. Left
		// unbounded it remembers whatever GitHub named, which can be minutes.
		//
		// That matters because a client is not per-call: sweepInstallation
		// mints one and reuses it for every repository in an installation. One
		// request tripping abuse detection would otherwise fail every
		// repository left in that sweep, instantly and silently, with the
		// maxRetryAfter cap never getting a look in.
		gogithub.WithMaxSecondaryRateLimitRetryAfterDuration(maxRetryAfter),
	)
}

// wrapError turns a go-github error into the APIError the rest of Smyklot
// already understands.
//
// Everything downstream keys on APIError: retryableDelivery asks Retryable(),
// mergeHeadChanged looks for a 409, and getFileContent turns a 404 into a nil
// result rather than an error. Mapping here rather than changing those callers
// is what lets the transport swap underneath them.
//
// The *http.Response go-github carries on its error types is deliberately not
// stored. It holds the request headers, and therefore the installation token,
// and an APIError reaches both the log and the /failures endpoint. The
// redactor only knows the three process-level secrets; an installation token is
// minted per delivery and can never be in that list, so the only safe handling
// is never to capture it.
func wrapError(op error, method, path string, err error) error {
	if err == nil {
		return nil
	}

	var (
		rateErr     *gogithub.RateLimitError
		abuseErr    *gogithub.AbuseRateLimitError
		respErr     *gogithub.ErrorResponse
		acceptedErr *gogithub.AcceptedError
	)

	switch {
	case errors.As(err, &rateErr):
		return newRetryableAPIError(
			op, statusOf(rateErr.Response, http.StatusForbidden), method, path,
			rateErr.Message,
		)

	case errors.As(err, &abuseErr):
		return newRetryableAPIError(
			op, statusOf(abuseErr.Response, http.StatusForbidden), method, path,
			abuseErr.Message,
		)

	case errors.As(err, &acceptedErr):
		// GitHub scheduled the work and has not finished it. Asking again is
		// exactly the documented remedy.
		return newRetryableAPIError(op, http.StatusAccepted, method, path, acceptedErr.Error())

	case errors.As(err, &respErr):
		// Message is the same field the hand-rolled client extracted, so the
		// substring heuristics in Retryable keep matching what they used to.
		return NewAPIError(
			op, statusOf(respErr.Response, 0), method, path, errors.New(detailOf(respErr)),
		)
	}

	// No status: the request never completed. That is already retryable by the
	// status-code rules, but saying so explicitly costs nothing and survives a
	// future change to them.
	return newRetryableAPIError(op, 0, method, path, err.Error())
}

// fromGitHub reports whether GitHub answered and this is what it said, rather
// than the answer being unreadable.
//
// One list, read by both wrapError and decodeOp. They each had their own and
// the two had already drifted: decodeOp was missing AcceptedError, so a 202 -
// "still working on it, ask again" - came back labelled a parse failure.
func fromGitHub(err error) bool {
	var (
		rateErr     *gogithub.RateLimitError
		abuseErr    *gogithub.AbuseRateLimitError
		respErr     *gogithub.ErrorResponse
		acceptedErr *gogithub.AcceptedError
	)

	return errors.As(err, &rateErr) ||
		errors.As(err, &abuseErr) ||
		errors.As(err, &respErr) ||
		errors.As(err, &acceptedErr)
}

func statusOf(resp *http.Response, fallback int) int {
	if resp == nil {
		return fallback
	}

	return resp.StatusCode
}

// detailOf prefers GitHub's per-field validation errors when it sent any.
// "Validation Failed" alone tells an operator nothing; "name already_exists"
// tells them which label collided.
func detailOf(err *gogithub.ErrorResponse) string {
	if len(err.Errors) == 0 {
		return err.Message
	}

	parts := make([]string, 0, len(err.Errors))
	for _, item := range err.Errors {
		detail := strings.TrimSpace(item.Field + " " + item.Code)
		if item.Message != "" {
			detail = strings.TrimSpace(item.Field + " " + item.Message)
		}

		if detail != "" {
			parts = append(parts, detail)
		}
	}

	if len(parts) == 0 {
		return err.Message
	}

	return err.Message + ": " + strings.Join(parts, "; ")
}

// doJSON sends one request through go-github's plumbing and decodes the answer
// into a type this package owns.
//
// It exists for the handful of responses where go-github's model loses
// something. The clearest case is an issue comment's updated_at: go-github
// parses it into a Timestamp, and every way of turning that back into a string
// is lossy in a way that matters here, because the value is an opaque revision
// token compared byte-for-byte against what a webhook payload carried.
// RFC3339 truncates sub-second precision and RFC3339Nano trims trailing zeros;
// either one turns "unchanged" into "changed" and makes Smyklot act twice.
//
// Everything else still comes from go-github: the base URL, the auth transport,
// the rate-limit accounting and the typed errors wrapError reads.
func doJSON[T any](
	ctx context.Context,
	client *Client,
	method, path string,
	body any,
) (T, error) {
	out, _, err := doJSONPage[T](ctx, client, method, path, body)

	return out, err
}

// paginate walks every page of a go-github list endpoint.
//
// The package had four hand-rolled versions of this loop and they did not agree
// with each other: some ended on a short page, some on a page count, one on the
// Link header. Reactions and reviews had no loop at all, so a comment with more
// than thirty reactions reported a truncated set and the code that removes a
// reaction quietly left some behind.
//
// fetch is called once per page and reports the page and the response carrying
// the Link header.
func paginate[T any](
	ctx context.Context,
	op string,
	fetch func(context.Context, *gogithub.ListOptions) ([]T, *gogithub.Response, error),
) ([]T, error) {
	// Page is set explicitly rather than left at zero. GitHub reads an absent
	// page as the first one, so both spellings work - but the hand-rolled
	// loops this replaces all sent it, and a request that changes shape is a
	// request a spec has to be rewritten to accept.
	opts := &gogithub.ListOptions{Page: 1, PerPage: pageSize}
	items := make([]T, 0, pageSize)

	for range maxPages {
		page, resp, err := fetch(ctx, opts)
		if err != nil {
			// decodeOp, not a bare ErrAPIRequest: a body that will not parse
			// is a permanent failure, and reporting it as a request failure
			// would put it back in the delivery retry queue eight times.
			return nil, wrapError(decodeOp(resp, err), http.MethodGet, op, err)
		}

		items = append(items, page...)

		// GitHub sends a next link while there is more. A short page is the
		// belt to that braces: an endpoint or proxy that omits Link entirely
		// would otherwise look like a single page and silently truncate.
		switch {
		case resp != nil && resp.NextPage > 0:
			opts.Page = resp.NextPage
		case len(page) < pageSize:
			return items, nil
		default:
			opts.Page++
		}
	}

	return nil, NewAPIError(ErrIncompletePagination, 0, http.MethodGet, op, nil)
}

// doRequest sends one request and discards the response body.
//
// For the endpoints whose answer carries nothing a caller needs: adding a
// reaction, deleting a comment, dismissing a review.
func doRequest(ctx context.Context, client *Client, method, path string, body any) error {
	req, err := newAPIRequest(ctx, client, method, path, body)
	if err != nil {
		return err
	}

	_, err = client.gh.Do(req, nil)

	return wrapError(ErrAPIRequest, method, path, err)
}

// newAPIRequest builds a request against the client's base URL.
//
// The paths in this package are written with a leading slash, the way the old
// hand-rolled client concatenated them, while go-github resolves each endpoint
// as a reference relative to a base that already ends in one.
func newAPIRequest(
	ctx context.Context,
	client *Client,
	method, path string,
	body any,
) (*http.Request, error) {
	req, err := client.gh.NewRequest(ctx, method, strings.TrimPrefix(path, "/"), body)
	if err != nil {
		return nil, NewAPIError(ErrAPIRequest, 0, method, path, err)
	}

	return req, nil
}

// doJSONPage is doJSON plus the response, for the callers that have to follow
// GitHub's Link header themselves.
func doJSONPage[T any](
	ctx context.Context,
	client *Client,
	method, path string,
	body any,
) (T, *gogithub.Response, error) {
	var out T

	req, err := newAPIRequest(ctx, client, method, path, body)
	if err != nil {
		return out, nil, err
	}

	resp, err := client.gh.Do(req, &out)
	if err != nil {
		return out, resp, wrapError(decodeOp(resp, err), method, path, err)
	}

	return out, resp, nil
}

// decodeOp separates "GitHub refused" from "GitHub answered something this
// package could not read".
//
// The distinction is load-bearing rather than cosmetic: ErrAPIRequest can be
// retryable, and a body that will not parse will not parse on the second
// attempt either. go-github reports a decode failure as a plain error against a
// successful response, which is exactly the pair checked here.
func decodeOp(resp *gogithub.Response, err error) error {
	if resp == nil || resp.Response == nil {
		return ErrAPIRequest
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ErrAPIRequest
	}

	if fromGitHub(err) {
		return ErrAPIRequest
	}

	return ErrResponseParse
}

// newRetryableAPIError marks an error retryable on GitHub's own authority
// rather than by inference from its status code.
func newRetryableAPIError(op error, statusCode int, method, path, detail string) error {
	retryable := true

	return &APIError{
		Op:         op,
		StatusCode: statusCode,
		Method:     method,
		Path:       path,
		Detail:     detail,
		retryable:  &retryable,
	}
}
