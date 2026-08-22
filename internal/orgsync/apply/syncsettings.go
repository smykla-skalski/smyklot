package apply

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
	//
	// A switch with a default rather than an if with a fallthrough, which is
	// how the label and ruleset executors answer the same "which
	// sub-instruction is this" question. The two spellings differ on the case
	// that does not exist yet: a third subject falls through an if and is sent
	// to GitHub as a settings body, and is refused by name here.
	switch action.Subject {
	case orgsync.DependabotSubject:
		change, err := orgsync.DecodeDependabot(action.Payload)
		if err != nil {
			return err
		}

		return client.SetAutomatedSecurityFixes(ctx, owner, name, change.Enabled)

	case orgsync.SettingsSubject:
		body, err := orgsync.DecodeSettings(action.Payload)
		if err != nil {
			return err
		}

		return client.UpdateRepositorySettings(ctx, owner, name, body)

	default:
		return fmt.Errorf("%w: settings %q", errSyncSubjectUnknown, action.Subject)
	}
}
