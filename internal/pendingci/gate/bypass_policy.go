package gate

import (
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func applyBypassPolicy(desired *github.RepositoryRuleset, policy *storage.PendingCIBypassPolicy) {
	if policy == nil {
		return
	}
	// Non-nil, including an empty list, means this policy owns the exceptions.
	desired.BypassActors = []github.RulesetBypassActor{}
	if !policy.Allow {
		return
	}
	for _, actor := range policy.Actors {
		id := actor.ActorID
		if actor.ActorType == "OrganizationAdmin" || actor.ActorType == "DeployKey" {
			id = 0
		}
		desired.BypassActors = append(desired.BypassActors, github.RulesetBypassActor{
			ActorID: id, ActorType: actor.ActorType, Mode: actor.Mode,
		})
	}
}
