package webhook

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	// unknownDeliveryID is what an unusable X-GitHub-Delivery becomes. The
	// header is not covered by the signature, so its value is scrubbed before
	// it reaches a log line, a metric label or a database row.
	unknownDeliveryID = "unknown"

	// maxDeliveryIDLength bounds it too. GitHub sends a UUID; anything longer
	// is somebody else's idea.
	maxDeliveryIDLength = 64

	// eventOther is the label an event outside the configured set is counted
	// under, so a made-up header cannot mint a time series per request.
	eventOther = "other"
)

// Delivery is one signature-verified webhook, on its way to a handler.
//
// Payload is the raw body rather than a decoded struct, and that is
// deliberate: it is what gets persisted, so a delivery claimed by one binary is
// decoded by whichever binary later leases it. A struct written to the inbox
// would have to survive every future change to the field it was written from.
type Delivery struct {
	// Event is X-GitHub-Event, reduced to a value safe to use as a metric
	// label or a log field.
	Event string

	// ID is X-GitHub-Delivery, similarly reduced. "unknown" when the header
	// carried nothing usable.
	ID string

	// Source is the installation and repository the payload names, and the
	// action it reports.
	Source Source

	Payload []byte

	// Key is the identity the delivery was deduplicated under. A handler that
	// keeps its own per-delivery state keys it by this, so a redelivery and a
	// retry agree on what they are.
	Key string

	// ClaimID is the inbox's handle on this delivery.
	ClaimID int64

	// Attempt starts at one and rises with every lease.
	Attempt int

	// Logger already carries this delivery's identifiers, so a handler does not
	// have to attach them again to be traceable.
	Logger *slog.Logger
}

// Outcome is how a delivery left the pipeline.
type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeRetrying  Outcome = "retrying"
)

// Request outcomes, reported to Observer.Received.
//
// The strings match what this repository's Prometheus collectors already
// use, so swapping the pipeline in does not rename anybody's time series.
const (
	OutcomeAccepted  = "accepted"
	OutcomeDuplicate = "duplicate"
	OutcomeIgnored   = "ignored"
	OutcomeInvalid   = "invalid"
	OutcomeRefused   = "refused"
	OutcomeUnsigned  = "unsigned"
)

// Retryable decides whether a failed attempt earns another, and how long to
// wait first.
type Retryable func(err error, attempt int) (delay time.Duration, again bool)

// Retry defaults.
const (
	retryBaseDelay = 2 * time.Second
	retryMaxDelay  = 5 * time.Minute
	maxAttempts    = 8
)

// DefaultRetry doubles from two seconds and gives up after eight attempts,
// which is a little over four minutes of trying.
//
// retryMaxDelay is a ceiling those eight attempts never reach - the seventh and
// last delay is 128 seconds. It is there so that raising the attempt budget
// cannot silently turn the backoff into hours.
//
// An error that implements Retryable() bool is asked. Anything else is assumed
// transient, because the failure that costs a user their command is the one
// that was retryable and was not retried.
func DefaultRetry(err error, attempt int) (time.Duration, bool) {
	if attempt >= maxAttempts {
		return 0, false
	}

	var classified interface{ Retryable() bool }
	if errors.As(err, &classified) && !classified.Retryable() {
		return 0, false
	}

	delay := retryBaseDelay
	for index := 1; index < attempt && delay < retryMaxDelay; index++ {
		delay *= 2
	}
	if delay > retryMaxDelay {
		delay = retryMaxDelay
	}

	return delay, true
}

// sanitizeDeliveryID reduces X-GitHub-Delivery to something safe to store.
//
// The signature covers the body and not the headers, so this value is whatever
// the caller sent. Everything but the alphabet GitHub's own GUIDs use is
// dropped, and a header that survives as nothing becomes "unknown".
func sanitizeDeliveryID(id string) string {
	if len(id) > maxDeliveryIDLength {
		id = id[:maxDeliveryIDLength]
	}

	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return -1
		}
	}, id)

	if clean == "" {
		return unknownDeliveryID
	}

	return clean
}

// eventLabel reduces X-GitHub-Event to one of the names the pipeline was
// configured for, or "other".
//
// Never use the raw header: it is unsigned and unbounded, and one made-up value
// per request is one time series per request.
func eventLabel(name string, known map[string]struct{}) string {
	if _, ok := known[name]; ok {
		return name
	}
	if name == EventPing {
		return name
	}

	return eventOther
}

// claimKey identifies a delivery for deduplication.
//
// GitHub's delivery GUID is the real identity - GitHub documents that a
// redelivery keeps it. The digest is the fallback for a caller that posts
// without the header, which in practice means a test.
func claimKey(event, deliveryID string, body []byte) string {
	if deliveryID == "" || deliveryID == unknownDeliveryID {
		return fmt.Sprintf("%s:sha256:%x", event, sha256.Sum256(body))
	}

	return "github-delivery:" + event + ":" + deliveryID
}
