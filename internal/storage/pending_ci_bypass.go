package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

// PendingCIBypassPolicy owns the exceptions on the merge-after-CI ruleset.
// A nil workspace policy leaves existing GitHub exceptions unchanged. This
// preserves installations configured before the panel could manage exceptions.
// A nil repository policy inherits the workspace policy.
type PendingCIBypassPolicy struct {
	Allow  bool                         `json:"allow"`
	Actors []orgsync.RulesetBypassActor `json:"actors"`
}

func (policy PendingCIBypassPolicy) MarshalJSON() ([]byte, error) {
	type document PendingCIBypassPolicy
	value := document(policy)
	if value.Actors == nil {
		value.Actors = []orgsync.RulesetBypassActor{}
	}
	return json.Marshal(value)
}

func (policy *PendingCIBypassPolicy) UnmarshalJSON(data []byte) error {
	var value struct {
		Allow  *bool                         `json:"allow"`
		Actors *[]orgsync.RulesetBypassActor `json:"actors"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if value.Allow == nil || value.Actors == nil {
		return errors.New("bypass policy requires allow and an actors list")
	}
	next := PendingCIBypassPolicy{Allow: *value.Allow, Actors: *value.Actors}
	if err := next.Validate(); err != nil {
		return err
	}
	*policy = next
	return nil
}

func (policy *PendingCIBypassPolicy) Validate() error {
	if policy == nil {
		return nil
	}
	return orgsync.ValidateRulesetBypassActors(policy.Actors)
}

func (policy *PendingCIBypassPolicy) ValidateForTarget(kind TargetKind) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if policy == nil || kind == TargetOrganization {
		return nil
	}
	for _, actor := range policy.Actors {
		if actor.ActorType == "OrganizationAdmin" || actor.ActorType == "Team" {
			return fmt.Errorf("%s exceptions require an organization workspace", actor.ActorType)
		}
	}
	return nil
}

func EffectivePendingCIBypassPolicy(target Target, repository Repository) *PendingCIBypassPolicy {
	if repository.PendingCIBypassPolicyOverride != nil {
		return repository.PendingCIBypassPolicyOverride
	}
	return target.PendingCIBypassPolicyDefault
}
