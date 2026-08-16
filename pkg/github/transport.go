package github

import (
	"net/http"
)

// authTransport stamps every request with this client's credentials.
//
// go-github's WithAuthToken always emits the Bearer scheme. GitHub rejects the
// token scheme on app-level endpoints such as GET /app, and rejects nothing for
// installation tokens, so Bearer alone would work — but the scheme a client
// sends is part of what its specs assert, and losing that distinction would
// quietly remove the only signal separating an app client from an installation
// one. Carrying the scheme keeps newClient and NewAppClient meaningfully
// different rather than incidentally so.
type authTransport struct {
	base   http.RoundTripper
	scheme string
	token  string
}

// RoundTrip clones the request before touching its headers. A RoundTripper must
// not modify the request it is given: the caller may still be holding it, and
// retry logic above may send it again.
func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())

	scheme := t.scheme
	if scheme == "" {
		scheme = schemeToken
	}

	clone.Header.Set("Authorization", scheme+" "+t.token)
	clone.Header.Set("User-Agent", userAgent)

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	return base.RoundTrip(clone)
}
