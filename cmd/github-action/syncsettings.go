package main

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// applySettingsAction changes one repository's settings.
//
// Everything it sends is on the action, decided when the plan was computed and
// read by whoever approved it. Reading the repository again here and diffing
// afresh would apply what is true now rather than what was agreed - and against
// an endpoint that replaces what it is given, the difference between those two
// is somebody else's change being undone.
func applySettingsAction(
	ctx context.Context,
	client *github.Client,
	owner, name string,
	action orgsync.Action,
) error {
	if action.Operation != orgsync.OperationUpdate {
		// Settings are not created or removed; a repository always has them.
		return fmt.Errorf("%w: %s settings", errSyncOperationUnknown, action.Operation)
	}

	if len(action.Payload) == 0 {
		return fmt.Errorf("%w: settings for %s/%s", errSyncPayloadMissing, owner, name)
	}

	// Which request this is comes off the subject, which is what the planner
	// decided and what the plan showed. Reading the payload's shape instead
	// would let a settings body with an "enabled" key in it reach the wrong
	// endpoint.
	if action.Subject == orgsync.DependabotSubject {
		change, err := orgsync.DecodeDependabot(action.Payload)
		if err != nil {
			return err
		}

		return client.SetAutomatedSecurityFixes(ctx, owner, name, change.Enabled)
	}

	body, err := orgsync.DecodeSettings(action.Payload)
	if err != nil {
		return err
	}

	return client.UpdateRepositorySettings(ctx, owner, name, body)
}
