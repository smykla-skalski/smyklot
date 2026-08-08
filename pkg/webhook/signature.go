package webhook

import (
	"net/http"

	"github.com/jferrl/go-githubauth/webhook"
)

// Signature verification errors, matched with errors.Is.
var (
	// ErrMissingSignature is returned when a delivery carries no signature
	ErrMissingSignature = webhook.ErrMissingSignature

	// ErrInvalidSignatureFormat is returned when the signature header is not
	// GitHub's "sha256=<hex>" form
	ErrInvalidSignatureFormat = webhook.ErrInvalidSignatureFormat

	// ErrSignatureMismatch is returned when the signature does not match the
	// body, whether because the body was tampered with or the secret is wrong
	ErrSignatureMismatch = webhook.ErrSignatureMismatch
)

// MiddlewareOpt configures Middleware.
type MiddlewareOpt = webhook.MiddlewareOpt

// Verify reports whether signature is a valid HMAC-SHA256 of body under secret,
// comparing in constant time.
func Verify(secret, body []byte, signature string) error {
	return webhook.Verify(secret, body, signature)
}

// Middleware returns net/http middleware that verifies a delivery's signature
// before the handler runs.
//
// A missing or wrong signature short-circuits with 401 and the handler never
// sees the body, so an unsigned delivery cannot change anything. A verified
// body is restored for the handler to read.
func Middleware(secret []byte, opts ...MiddlewareOpt) func(http.Handler) http.Handler {
	return webhook.Middleware(secret, opts...)
}

// WithErrorHandler overrides how a verification failure is reported.
func WithErrorHandler(fn func(http.ResponseWriter, *http.Request, error)) MiddlewareOpt {
	return webhook.WithErrorHandler(fn)
}
