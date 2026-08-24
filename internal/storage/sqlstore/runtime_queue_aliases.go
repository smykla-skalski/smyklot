package sqlstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
)

func (s *Store) syncRuntimeQueueAliases(
	ctx context.Context,
	tx *transaction,
	current storage.RuntimeSettings,
	change storage.RuntimeSettingsChange,
) error {
	pollChanged := !sameOptionalDuration(current.PollInterval, change.PollInterval)
	if pollChanged {
		cadence := effectiveAliasDuration(change.PollInterval, change.EffectivePollInterval)
		for _, kind := range []workqueue.Kind{
			workqueue.KindReactionScan,
			workqueue.KindConfigMigration,
			workqueue.KindPendingCIGate,
			workqueue.KindSyncScan,
		} {
			if err := s.updateRuntimeCadenceAlias(
				ctx, tx, kind, cadence, true, change,
			); err != nil {
				return err
			}
		}
	}
	pathChanged := !sameOptionalDuration(current.PathIndexInterval, change.PathIndexInterval)
	if pathChanged || (pollChanged && change.EffectivePathIndexInterval == 0) {
		pathInterval := effectiveAliasDuration(
			change.PathIndexInterval,
			change.EffectivePathIndexInterval,
		)
		cadence := pathInterval
		if pathInterval == 0 {
			cadence = change.EffectivePollInterval
		}
		if err := s.updateRuntimeCadenceAlias(
			ctx, tx, workqueue.KindPathRefresh,
			cadence, true, change,
		); err != nil {
			return err
		}
	}
	if !sameOptionalDuration(current.PendingCIQuietPeriod, change.PendingCIQuietPeriod) {
		if err := s.updatePendingCIQueueAlias(ctx, tx, change); err != nil {
			return err
		}
	}

	return nil
}

func effectiveAliasDuration(value *time.Duration, fallback time.Duration) time.Duration {
	if value != nil {
		return *value
	}

	return fallback
}

func (s *Store) updateRuntimeCadenceAlias(
	ctx context.Context,
	tx *transaction,
	kind workqueue.Kind,
	cadence time.Duration,
	disableAtZero bool,
	change storage.RuntimeSettingsChange,
) error {
	policy, err := getEffectiveQueuePolicy(ctx, tx, kind, nil)
	if err != nil {
		return err
	}
	policy.Cadence = cadence
	policy.Enabled = !disableAtZero || cadence > 0

	return s.replaceRuntimeAliasPolicy(ctx, tx, policy, change)
}

func (s *Store) updatePendingCIQueueAlias(
	ctx context.Context,
	tx *transaction,
	change storage.RuntimeSettingsChange,
) error {
	policy, err := getEffectiveQueuePolicy(ctx, tx, workqueue.KindPendingCI, nil)
	if err != nil {
		return err
	}
	configuration := map[string]json.RawMessage{}
	if err := json.Unmarshal(policy.Configuration, &configuration); err != nil {
		return fmt.Errorf("decode pending CI queue policy: %w", err)
	}
	quietPeriod := effectiveAliasDuration(
		change.PendingCIQuietPeriod,
		change.EffectivePendingCIQuietPeriod,
	)
	quiet, err := json.Marshal(int64(quietPeriod / time.Second))
	if err != nil {
		return err
	}
	configuration["passing_quiet_seconds"] = quiet
	policy.Configuration, err = json.Marshal(configuration)
	if err != nil {
		return fmt.Errorf("encode pending CI queue policy: %w", err)
	}

	return s.replaceRuntimeAliasPolicy(ctx, tx, policy, change)
}

func (s *Store) replaceRuntimeAliasPolicy(
	ctx context.Context,
	tx *transaction,
	policy workqueue.Policy,
	change storage.RuntimeSettingsChange,
) error {
	policyChange := workqueue.PolicyChange{
		Kind: policy.Kind, Enabled: policy.Enabled, Cadence: policy.Cadence,
		ProfileID: policy.ProfileID, DefaultPriority: policy.DefaultPriority,
		RetryDelay: policy.RetryDelay, Retention: policy.Retention,
		ApprovalTTL: policy.ApprovalTTL, Configuration: policy.Configuration,
		ExpectedRevision: policy.Revision, ActorID: change.ActorAccountID,
		ChangedAt: change.ChangedAt,
	}
	if err := saveQueuePolicy(ctx, tx, policyChange); err != nil {
		return fmt.Errorf("update runtime queue alias %s: %w", policy.Kind, err)
	}
	updated, err := getEffectiveQueuePolicy(ctx, tx, policy.Kind, nil)
	if err != nil {
		return err
	}
	profile, err := getScheduleProfile(ctx, tx, updated.ProfileID)
	if err != nil {
		return err
	}

	return s.reschedulePolicyItems(
		ctx, tx, updated, profile, change.ChangedAt,
		change.ActorAccountID, "Legacy runtime alias changed the queue policy",
	)
}
