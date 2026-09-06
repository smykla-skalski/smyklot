package configsync

import (
	"context"
	"errors"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

func (engine Engine) settle(
	ctx context.Context, client *github.Client, snapshot PanelSnapshot, stored storage.ConfigFileState,
	connection Connection, file RemoteFile, semantic []byte,
) (Connection, bool, error) {
	var pull *github.PullRequest
	if proposal := connection.Proposal; proposal != nil {
		// A lost creation response may leave no PR number, and a human can
		// retarget a proposal. Search open proposals across every base branch.
		// A previous closed PR must not hide another open one from this branch.
		var err error
		pull, err = readOutstandingProposal(ctx, client, file, *proposal)
		if err != nil {
			if errors.Is(err, storage.ErrConflict) {
				return connection, true, nil
			}
			return engine.block(ctx, snapshot, stored, connection, err)
		}
	}
	// Agreement advances the common baseline even when a separate proposal
	// still needs review. Otherwise a later panel revert looks like a file edit.
	connection.Base = Snapshot{Exists: true, Document: semantic}
	connection.Resolution = nil
	if pull != nil {
		connection.Proposal.Number, connection.Proposal.URL = pull.Number, pull.URL
		// GitHub cannot condition a close on an unchanged PR head. Preserve
		// concurrent human work and keep the outstanding proposal visible.
		return engine.block(ctx, snapshot, stored, connection, outstandingProposal())
	}
	connection.Status = StatusReady
	return engine.save(ctx, snapshot, stored, connection)
}

func readOutstandingProposal(ctx context.Context, client *github.Client, file RemoteFile, proposal Proposal) (*github.PullRequest, error) {
	pull, err := client.FindOpenPullRequestByHead(ctx, file.Location.Owner, file.Location.Repository, proposal.Branch)
	if err == nil {
		// A proposal can merge while we check it. Do not report agreement using
		// the file read before that merge or another repository rename.
		err = requirePublicationSource(ctx, client, file.Location, file.Head)
	}
	var blocked *BlockedError
	if errors.As(err, &blocked) && blocked.Code == sourceChanged {
		return nil, storage.ErrConflict
	}
	return pull, err
}

func outstandingProposal() *BlockedError {
	return &BlockedError{Code: "proposal_outstanding", Message: "Saved settings match the file, but a configuration pull request is still open; review it on GitHub"}
}

func (engine Engine) previewStatus(ctx context.Context, client *github.Client, observation reviewObservation, preview ConnectionPreview) (ConnectionPreview, error) {
	if preview.Status != StatusReady || observation.connection.Proposal == nil {
		return preview, nil
	}
	pull, err := readOutstandingProposal(ctx, client, observation.file, *observation.connection.Proposal)
	if err != nil {
		return ConnectionPreview{}, err
	}
	if err := engine.verifyReview(ctx, client, observation); err != nil {
		return ConnectionPreview{}, err
	}
	if pull != nil {
		problem := outstandingProposal()
		preview.Status, preview.Problem, preview.Message = StatusBlocked, problem.Code, problem.Message
		preview.Proposal = &CheckedProposal{Number: pull.Number, URL: pull.URL}
	}
	return preview, nil
}
