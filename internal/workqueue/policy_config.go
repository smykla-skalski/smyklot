package workqueue

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PendingCITiming is the job-specific part of the pending-CI policy. Durations
// stay in the domain as durations even though the persisted JSON uses seconds.
type PendingCITiming struct {
	ActiveInterval   time.Duration
	DiscoveryGrace   time.Duration
	DeferAfter       time.Duration
	DeferredInterval time.Duration
	PassingQuiet     time.Duration
}

type pendingCITimingDocument struct {
	ActiveCheckSeconds   int64 `json:"active_check_seconds"`
	NoCheckGraceSeconds  int64 `json:"no_check_grace_seconds"`
	DeferAfterSeconds    int64 `json:"defer_after_seconds"`
	DeferredCheckSeconds int64 `json:"deferred_check_seconds"`
	PassingQuietSeconds  int64 `json:"passing_quiet_seconds"`
}

func ParsePendingCITiming(raw json.RawMessage, fallback PendingCITiming) (PendingCITiming, error) {
	if len(raw) == 0 {
		return fallback, nil
	}
	document := pendingCITimingDocument{
		ActiveCheckSeconds:   durationSeconds(fallback.ActiveInterval),
		NoCheckGraceSeconds:  durationSeconds(fallback.DiscoveryGrace),
		DeferAfterSeconds:    durationSeconds(fallback.DeferAfter),
		DeferredCheckSeconds: durationSeconds(fallback.DeferredInterval),
		PassingQuietSeconds:  durationSeconds(fallback.PassingQuiet),
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return PendingCITiming{}, fmt.Errorf("decode pending-CI timing: %w", err)
	}
	parsed := PendingCITiming{
		ActiveInterval:   secondsDuration(document.ActiveCheckSeconds),
		DiscoveryGrace:   secondsDuration(document.NoCheckGraceSeconds),
		DeferAfter:       secondsDuration(document.DeferAfterSeconds),
		DeferredInterval: secondsDuration(document.DeferredCheckSeconds),
		PassingQuiet:     secondsDuration(document.PassingQuietSeconds),
	}
	if parsed.ActiveInterval <= 0 || parsed.DiscoveryGrace <= 0 ||
		parsed.DeferAfter <= 0 || parsed.DeferredInterval <= 0 || parsed.PassingQuiet < 0 {
		return PendingCITiming{}, errors.New("pending-CI timing values are out of range")
	}

	return parsed, nil
}

// WebhookRetry is the configurable exponential retry budget for delivery
// failures. RetryDelay on the policy is the base delay; this document owns the
// cap and attempt budget.
type WebhookRetry struct {
	MaxDelay    time.Duration
	MaxAttempts int
}

type webhookRetryDocument struct {
	MaxDelaySeconds int64 `json:"max_delay_seconds"`
	MaxAttempts     int   `json:"max_attempts"`
}

func ParseWebhookRetry(raw json.RawMessage, fallback WebhookRetry) (WebhookRetry, error) {
	if len(raw) == 0 {
		return fallback, nil
	}
	document := webhookRetryDocument{
		MaxDelaySeconds: durationSeconds(fallback.MaxDelay),
		MaxAttempts:     fallback.MaxAttempts,
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return WebhookRetry{}, fmt.Errorf("decode webhook retry timing: %w", err)
	}
	parsed := WebhookRetry{
		MaxDelay: secondsDuration(document.MaxDelaySeconds), MaxAttempts: document.MaxAttempts,
	}
	if parsed.MaxDelay <= 0 || parsed.MaxAttempts < 1 || parsed.MaxAttempts > 100 {
		return WebhookRetry{}, errors.New("webhook retry values are out of range")
	}

	return parsed, nil
}

func ValidatePolicyConfiguration(kind Kind, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if !json.Valid(raw) {
		return errors.New("queue policy configuration must be valid JSON")
	}
	switch kind {
	case KindPendingCI:
		_, err := ParsePendingCITiming(raw, PendingCITiming{
			ActiveInterval: time.Second, DiscoveryGrace: time.Second,
			DeferAfter: time.Second, DeferredInterval: time.Second,
		})

		return err
	case KindWebhookDelivery:
		_, err := ParseWebhookRetry(raw, WebhookRetry{MaxDelay: time.Second, MaxAttempts: 1})

		return err
	default:
		var document map[string]json.RawMessage
		if err := json.Unmarshal(raw, &document); err != nil {
			return errors.New("queue policy configuration must be a JSON object")
		}

		return nil
	}
}

func durationSeconds(value time.Duration) int64 { return int64(value / time.Second) }

func secondsDuration(value int64) time.Duration { return time.Duration(value) * time.Second }
