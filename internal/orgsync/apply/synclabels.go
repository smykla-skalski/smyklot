package apply

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var (
	errSyncOperationUnknown = errors.New("unknown sync operation")

	// errSyncSubjectUnknown is an action whose subject names no request this
	// version can make. Like the operation above it, it cannot come from a plan
	// this version computed - a kind whose actions are told apart by subject
	// refuses an unknown one rather than falling through to whichever branch
	// happens to be last.
	errSyncSubjectUnknown = errors.New("unknown sync subject")

	// errSyncPayloadMissing is an action that says to write something without
	// saying what. It cannot happen from a plan this version computed, and it
	// is refused rather than guessed at: guessing would apply something nobody
	// approved, which is the one thing the plan exists to prevent.
	errSyncPayloadMissing = errors.New("the plan does not say what to apply")
)

// applyLabelAction performs one label change.
//
// Everything it needs is on the action, because that is what somebody read and
// approved. Re-reading the configuration here would apply what it says now
// rather than what was agreed, which is the whole failure the plan and apply
// split exists to prevent.
func (s *Engine) applyLabelAction(
	ctx context.Context,
	client *github.Client,
	owner, name string,
	action orgsync.Action,
) error {
	if action.Operation == orgsync.OperationDelete {
		// A deletion carries no payload: the subject is the whole of the
		// instruction.
		return client.DeleteRepositoryLabel(ctx, owner, name, action.Subject)
	}

	if len(action.Payload) == 0 {
		return fmt.Errorf("%w: %s %q",
			errSyncPayloadMissing, action.Operation, action.Subject)
	}

	label, err := orgsync.DecodeLabel(action.Payload)
	if err != nil {
		return err
	}

	wanted := github.RepositoryLabel{
		Name: label.Name, Color: label.Color, Description: label.Description,
	}

	switch action.Operation {
	case orgsync.OperationCreate:
		return client.CreateRepositoryLabel(ctx, owner, name, wanted)

	case orgsync.OperationUpdate:
		// Addressed by the subject, which is the name the plan was computed
		// against. GitHub resolves a label path case-insensitively, so a
		// repository holding another spelling is still found, and new_name is
		// what settles it.
		return client.UpdateRepositoryLabel(ctx, owner, name, action.Subject, wanted)

	default:
		return fmt.Errorf("%w: %s", errSyncOperationUnknown, action.Operation)
	}
}
