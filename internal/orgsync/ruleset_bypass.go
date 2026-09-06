package orgsync

const bypassActorDeployKey = "DeployKey"

// ValidateRulesetBypassActors is shared by Sync rulesets and merge-after-CI
// settings so both writers enforce the same GitHub contract.
func ValidateRulesetBypassActors(actors []RulesetBypassActor) error {
	seen := make(map[string]map[int64]bool)
	for _, actor := range actors {
		if err := actor.validate("bypass exceptions"); err != nil {
			return err
		}
		id := actor.ActorID
		if actor.ActorType == bypassActorOrganizationAdmin || actor.ActorType == bypassActorDeployKey {
			id = 0
		}
		if seen[actor.ActorType] == nil {
			seen[actor.ActorType] = make(map[int64]bool)
		}
		if seen[actor.ActorType][id] {
			return invalid("bypass actor %s is listed more than once", actor.ActorType)
		}
		seen[actor.ActorType][id] = true
	}

	return nil
}
