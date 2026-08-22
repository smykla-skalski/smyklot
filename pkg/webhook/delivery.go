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
	unknownDeliveryID   = "unknown"
	maxDeliveryIDLength = 64
	eventOther          = "other"
)

type Delivery struct {
	Event   string
	ID      string
	Source  Source
	Payload []byte
	Key     string
	ClaimID int64
	Attempt int
	Logger  *slog.Logger
}

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeRetrying  Outcome = "retrying"
)

const (
	OutcomeAccepted  = "accepted"
	OutcomeDuplicate = "duplicate"
	OutcomeIgnored   = "ignored"
	OutcomeInvalid   = "invalid"
	OutcomeRefused   = "refused"
	OutcomeUnsigned  = "unsigned"
)

type Retryable func(err error, attempt int) (delay time.Duration, again bool)

const (
	retryBaseDelay = 2 * time.Second
	retryMaxDelay  = 5 * time.Minute
	maxAttempts    = 8
)

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

func eventLabel(name string, known map[string]struct{}) string {
	if _, ok := known[name]; ok {
		return name
	}
	if name == EventPing {
		return name
	}

	return eventOther
}

func claimKey(event, deliveryID string, body []byte) string {
	if deliveryID == "" || deliveryID == unknownDeliveryID {
		return fmt.Sprintf("%s:sha256:%x", event, sha256.Sum256(body))
	}

	return "github-delivery:" + event + ":" + deliveryID
}
