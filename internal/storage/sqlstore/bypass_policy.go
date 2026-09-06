package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func validateInstallationBypassPolicies(ctx context.Context, tx *transaction, prepared preparedInstallationSettings) error {
	var policies []*storage.PendingCIBypassPolicy
	if prepared.target != nil && prepared.target.change.PendingCIBypassPolicyDefault != nil {
		policies = append(policies, prepared.target.change.PendingCIBypassPolicyDefault)
	}
	for _, repository := range prepared.repositories {
		if repository.change.PendingCIBypassPolicyOverride != nil {
			policies = append(policies, repository.change.PendingCIBypassPolicyOverride)
		}
	}
	if len(policies) == 0 {
		return nil
	}
	var kind storage.TargetKind
	if err := tx.QueryRowContext(ctx, "SELECT kind FROM targets WHERE id = ?", prepared.request.TargetID).Scan(&kind); err != nil {
		return fmt.Errorf("read bypass policy scope: %w", noRows(err))
	}
	for _, policy := range policies {
		if err := policy.ValidateForTarget(kind); err != nil {
			return err
		}
	}
	return nil
}

func marshalBypassPolicy(policy *storage.PendingCIBypassPolicy) (any, error) {
	if policy == nil {
		return nil, nil
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("encode bypass policy: %w", err)
	}

	return string(data), nil
}

func unmarshalBypassPolicy(value sql.NullString) (*storage.PendingCIBypassPolicy, error) {
	if !value.Valid {
		return nil, nil
	}
	var policy storage.PendingCIBypassPolicy
	if err := json.Unmarshal([]byte(value.String), &policy); err != nil {
		return nil, fmt.Errorf("decode bypass policy: %w", err)
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}

	return &policy, nil
}
