package workqueue

import (
	"encoding/json"
	"strconv"
	"time"
)

const (
	defaultCatalogCadence = 5 * time.Minute
	defaultSyncCadence    = 6 * time.Hour
	defaultCleanupCadence = 5 * time.Minute
)

// DeploymentDefaults are the process-level timings that existed before the
// durable queue. They seed pristine queue policies without replacing later
// Root edits.
type DeploymentDefaults struct {
	PollInterval         time.Duration
	PendingCIQuietPeriod time.Duration
	PathIndexInterval    time.Duration
}

// DeploymentPolicies returns the complete immutable deployment policy set.
// Positive sub-second legacy intervals round up because the durable ledger
// persists schedule instants at whole-second precision.
func DeploymentPolicies(defaults DeploymentDefaults) []Policy {
	pollCadence := durableCadence(defaults.PollInterval)
	pathCadence := durableCadence(defaults.PathIndexInterval)
	if defaults.PathIndexInterval == 0 {
		pathCadence = pollCadence
	}
	retention := 30 * 24 * time.Hour
	approvalTTL := 2 * time.Hour
	epoch := time.Unix(0, 0).UTC()
	policy := func(
		kind Kind,
		enabled bool,
		cadence time.Duration,
		priority Priority,
		retry time.Duration,
		configuration string,
	) Policy {
		return Policy{
			Kind: kind, Enabled: enabled, Cadence: cadence,
			ProfileID: AlwaysOpenProfileID, DefaultPriority: priority,
			RetryDelay: retry, Configuration: json.RawMessage(configuration),
			Revision: 1, UpdatedAt: epoch,
		}
	}

	webhook := policy(
		KindWebhookDelivery, true, 0, PriorityUrgent, 2*time.Second,
		`{"max_delay_seconds":300,"max_attempts":8}`,
	)
	webhook.Retention = &retention
	pendingCI := policy(
		KindPendingCI, true, 5*time.Minute, PriorityNormal, 5*time.Second,
		pendingCIConfiguration(defaults.PendingCIQuietPeriod),
	)
	syncScan := policy(
		KindSyncScan, true, defaultSyncCadence, PriorityNormal, 5*time.Minute, `{}`,
	)
	syncScan.ApprovalTTL = &approvalTTL
	deliveryCleanup := policy(
		KindDeliveryCleanup, true, defaultCleanupCadence, PriorityLow, 5*time.Minute, `{}`,
	)
	deliveryCleanup.Retention = &retention

	return []Policy{
		webhook,
		pendingCI,
		policy(KindPendingCIGate, pollCadence > 0, pollCadence, PriorityNormal, 30*time.Second, `{}`),
		policy(KindCatalogRefresh, true, defaultCatalogCadence, PriorityNormal, 30*time.Second, `{}`),
		policy(KindReactionScan, pollCadence > 0, pollCadence, PriorityNormal, 30*time.Second, `{}`),
		policy(KindConfigMigration, pollCadence > 0, pollCadence, PriorityNormal, 30*time.Second, `{}`),
		syncScan,
		policy(KindSyncApply, true, 0, PriorityNormal, 5*time.Minute, `{}`),
		policy(KindPathRefresh, pathCadence > 0, pathCadence, PriorityLow, 5*time.Minute, `{}`),
		deliveryCleanup,
		policy(KindAuthCleanup, true, defaultCleanupCadence, PriorityLow, 5*time.Minute, `{}`),
	}
}

func durableCadence(value time.Duration) time.Duration {
	if value <= 0 {
		return 0
	}
	seconds := (value + time.Second - 1) / time.Second

	return seconds * time.Second
}

func pendingCIConfiguration(quiet time.Duration) string {
	return `{"active_check_seconds":300,"no_check_grace_seconds":600,` +
		`"defer_after_seconds":3600,"deferred_check_seconds":21600,` +
		`"passing_quiet_seconds":` + durationSecondsString(quiet) + `}`
}

func durationSecondsString(value time.Duration) string {
	return strconv.FormatInt(int64(value/time.Second), 10)
}
