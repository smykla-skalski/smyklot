package webhook

import "errors"

// Sentinel errors for webhook delivery handling.
var (
	// ErrMalformedPayload is returned when a delivery body is not valid JSON
	ErrMalformedPayload = errors.New("malformed webhook payload")

	// ErrNoInstallation is returned when a delivery carries no installation,
	// leaving nothing to mint a token for
	ErrNoInstallation = errors.New("webhook payload carries no installation")

	// ErrNoRepository is returned when a delivery carries no repository
	ErrNoRepository = errors.New("webhook payload carries no repository")
)
