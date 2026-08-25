package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

type actionPendingCIRepairCandidate struct {
	method       github.MergeMethod
	requiredOnly bool
	label        string
}

type actionPendingCIRepairMatch struct {
	candidate actionPendingCIRepairCandidate
	artifact  actionPendingCIArtifact
	found     bool
}

var actionPendingCIRepairCandidates = []actionPendingCIRepairCandidate{
	{method: github.MergeMethodMerge, label: LabelPendingCIMerge},
	{method: github.MergeMethodSquash, label: LabelPendingCISquash},
	{method: github.MergeMethodRebase, label: LabelPendingCIRebase},
	{method: github.MergeMethodMerge, requiredOnly: true, label: LabelPendingCIMergeRequired},
	{method: github.MergeMethodSquash, requiredOnly: true, label: LabelPendingCISquashRequired},
	{method: github.MergeMethodRebase, requiredOnly: true, label: LabelPendingCIRebaseRequired},
}

func recoverActionPendingCIRepairs(
	ctx context.Context,
	client *github.Client,
	botConfig *config.Config,
	owner, repository string,
	prs []map[string]interface{},
	botUsername string,
) []PendingCIPR {
	recovered := make([]PendingCIPR, 0)
	for _, pr := range prs {
		request, found, err := recoverActionPendingCIRepair(
			ctx, client, botConfig, owner, repository, pr, botUsername,
		)
		if err != nil {
			logging.From(ctx).Warn(
				"failed to recover pending-CI repair",
				"pr", ExtractPRNumber(pr), "error", err,
			)
		}
		if found {
			recovered = append(recovered, request)
		}
	}

	return recovered
}

func recoverActionPendingCIRepair(
	ctx context.Context,
	client *github.Client,
	botConfig *config.Config,
	owner, repository string,
	pr map[string]interface{},
	botUsername string,
) (PendingCIPR, bool, error) {
	pullRequest := ExtractPRNumber(pr)
	if pullRequest == 0 {
		return PendingCIPR{}, false, errors.New("invalid PR number")
	}
	repairSignal, err := actionPendingCIRepairSignal(
		ctx, client, owner, repository, pullRequest, botUsername,
	)
	if err != nil || repairSignal.ID == 0 {
		return PendingCIPR{}, false, err
	}
	state, err := client.GetPullRequestState(ctx, owner, repository, pullRequest)
	if err != nil {
		return PendingCIPR{}, false, fmt.Errorf("read pending-CI repair state: %w", err)
	}
	serviceOwned, err := pendingCIServiceOwnedForState(
		ctx, client, owner, repository, pullRequest, botUsername, state,
	)
	if err != nil {
		return PendingCIPR{}, false, err
	}
	if !state.Open || state.Draft || serviceOwned {
		err = clearActionPendingCIRepair(
			ctx, client, owner, repository, pullRequest, repairSignal.ID,
		)

		return PendingCIPR{}, false, err
	}
	match, err := latestRecoverableActionPendingCI(
		ctx, client, botConfig, owner, repository, pullRequest, botUsername,
	)
	if err != nil {
		return PendingCIPR{}, false, err
	}
	if !match.found {
		err = clearActionPendingCIRepair(
			ctx, client, owner, repository, pullRequest, repairSignal.ID,
		)

		return PendingCIPR{}, false, err
	}
	label := existingPendingCILabel(state.Labels, match.candidate)
	if label == "" {
		label = match.candidate.label
		if err = client.AddLabel(ctx, owner, repository, pullRequest, label); err != nil {
			return PendingCIPR{}, false, fmt.Errorf("restore pending-CI repair label: %w", err)
		}
	}
	request := PendingCIPR{
		PRData: pr, Method: match.candidate.method, Label: label,
		RequiredOnly: match.candidate.requiredOnly,
	}
	err = clearActionPendingCIRepair(
		ctx, client, owner, repository, pullRequest, repairSignal.ID,
	)

	return request, true, err
}

func latestRecoverableActionPendingCI(
	ctx context.Context,
	client *github.Client,
	botConfig *config.Config,
	owner, repository string,
	pullRequest int,
	botUsername string,
) (actionPendingCIRepairMatch, error) {
	var latest actionPendingCIRepairMatch
	for _, candidate := range actionPendingCIRepairCandidates {
		artifacts, _, err := actionPendingCIArtifacts(
			ctx, client, botConfig, owner, repository, pullRequest,
			candidate.method, candidate.requiredOnly, botUsername,
			actionPendingCIArtifactExclusion{},
		)
		if err != nil {
			return actionPendingCIRepairMatch{}, err
		}
		for _, artifact := range artifacts {
			valid, err := recoverableActionPendingCIArtifact(
				ctx, client, owner, repository, pullRequest, artifact,
			)
			if err != nil {
				return actionPendingCIRepairMatch{}, err
			}
			if valid && (!latest.found || newerActionPendingCIMarker(
				artifact.pending, latest.artifact.pending,
			)) {
				latest = actionPendingCIRepairMatch{
					candidate: candidate, artifact: artifact, found: true,
				}
			}
		}
	}

	return latest, nil
}

func recoverableActionPendingCIArtifact(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	artifact actionPendingCIArtifact,
) (bool, error) {
	if artifact.legacy || !artifact.bound {
		return false, nil
	}
	err := ValidateDraftMergeAuthorization(
		ctx, client, owner, repository, pullRequest, artifact.revision,
	)
	if errors.Is(err, pendingci.ErrStaleSourceRevision) ||
		errors.Is(err, pendingci.ErrAmbiguousSourceRevision) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func actionPendingCIRepairSignal(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	botUsername string,
) (github.Reaction, error) {
	reactions, err := client.GetPRReactions(ctx, owner, repository, pullRequest)
	if err != nil {
		return github.Reaction{}, fmt.Errorf("read pending-CI repair signal: %w", err)
	}
	var latest github.Reaction
	for _, reaction := range reactions {
		if reaction.User == botUsername && reaction.Type == ReactionPendingCIRepair &&
			newerActionPendingCIMarker(reaction, latest) {
			latest = reaction
		}
	}

	return latest, nil
}

func rotateActionPendingCIRepair(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	botUsername string,
) error {
	current, err := actionPendingCIRepairSignal(
		ctx, client, owner, repository, pullRequest, botUsername,
	)
	if err != nil {
		return err
	}
	// Rotate before publishing the retry so every repair that overlaps an
	// existing recovery gets a newer ID. Recoveries delete only the ID they
	// observed; they cannot consume this generation after it is created.
	if current.ID != 0 {
		if err = clearActionPendingCIRepair(
			ctx, client, owner, repository, pullRequest, current.ID,
		); err != nil {
			return fmt.Errorf("rotate pending-CI repair signal: %w", err)
		}
	}
	created, err := client.AddPullRequestReactionState(
		ctx, owner, repository, pullRequest, ReactionPendingCIRepair,
	)
	if err != nil {
		return fmt.Errorf("record pending-CI repair signal: %w", err)
	}
	if created.ID == 0 {
		return errors.New("GitHub returned an incomplete pending-CI repair signal")
	}

	return nil
}

func clearActionPendingCIRepair(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	reactionID int64,
) error {
	err := client.RemovePullRequestReaction(
		ctx, owner, repository, pullRequest, reactionID,
	)
	var apiErr *github.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil
	}

	return err
}

func existingPendingCILabel(
	labels []string,
	candidate actionPendingCIRepairCandidate,
) string {
	for _, label := range labels {
		method, requiredOnly, parsed := ParsePendingCILabel(label)
		if parsed != "" && method == candidate.method && requiredOnly == candidate.requiredOnly {
			return label
		}
	}

	return ""
}

func appendPendingCIRequest(requests []PendingCIPR, candidate PendingCIPR) []PendingCIPR {
	candidateNumber := ExtractPRNumber(candidate.PRData)
	for _, request := range requests {
		if ExtractPRNumber(request.PRData) == candidateNumber &&
			request.Method == candidate.Method &&
			request.RequiredOnly == candidate.RequiredOnly {
			return requests
		}
	}

	return append(requests, candidate)
}
