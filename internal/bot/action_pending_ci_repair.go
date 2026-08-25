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

type actionPendingCIRepairSignals struct {
	ready github.Reaction
	claim github.Reaction
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
	claim, acquired, err := acquireActionPendingCIRepair(
		ctx, client, owner, repository, pullRequest, botUsername,
	)
	if err != nil || !acquired {
		return PendingCIPR{}, false, err
	}
	state, err := client.GetPullRequestState(ctx, owner, repository, pullRequest)
	if err != nil {
		return retryActionPendingCIRepair(
			ctx, client, owner, repository, pullRequest,
			fmt.Errorf("read pending-CI repair state: %w", err),
		)
	}
	serviceOwned, err := pendingCIServiceOwnedForState(
		ctx, client, owner, repository, pullRequest, botUsername, state,
	)
	if err != nil {
		return retryActionPendingCIRepair(
			ctx, client, owner, repository, pullRequest, err,
		)
	}
	if !state.Open || state.Draft || serviceOwned {
		return PendingCIPR{}, false, clearActionPendingCIRepairClaim(
			ctx, client, owner, repository, pullRequest, claim.ID,
		)
	}
	match, err := latestRecoverableActionPendingCI(
		ctx, client, botConfig, owner, repository, pullRequest, botUsername,
	)
	if err != nil {
		return retryActionPendingCIRepair(
			ctx, client, owner, repository, pullRequest, err,
		)
	}
	if !match.found {
		return PendingCIPR{}, false, clearActionPendingCIRepairClaim(
			ctx, client, owner, repository, pullRequest, claim.ID,
		)
	}
	label := existingPendingCILabel(state.Labels, match.candidate)
	if label == "" {
		label = match.candidate.label
		if err = client.AddLabel(ctx, owner, repository, pullRequest, label); err != nil {
			return retryActionPendingCIRepair(
				ctx, client, owner, repository, pullRequest,
				fmt.Errorf("restore pending-CI repair label: %w", err),
			)
		}
	}
	request := PendingCIPR{
		PRData: pr, Method: match.candidate.method, Label: label,
		RequiredOnly: match.candidate.requiredOnly,
	}
	err = clearActionPendingCIRepairClaim(
		ctx, client, owner, repository, pullRequest, claim.ID,
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

func actionPendingCIRepairSignalsForPR(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	botUsername string,
) (actionPendingCIRepairSignals, error) {
	reactions, err := client.GetPRReactions(ctx, owner, repository, pullRequest)
	if err != nil {
		return actionPendingCIRepairSignals{},
			fmt.Errorf("read pending-CI repair signals: %w", err)
	}
	var signals actionPendingCIRepairSignals
	for _, reaction := range reactions {
		if reaction.User != botUsername {
			continue
		}
		if reaction.Type == ReactionPendingCIRepair &&
			newerActionPendingCIMarker(reaction, signals.ready) {
			signals.ready = reaction
		}
		if reaction.Type == ReactionPendingCIRepairClaim &&
			newerActionPendingCIMarker(reaction, signals.claim) {
			signals.claim = reaction
		}
	}

	return signals, nil
}

func acquireActionPendingCIRepair(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	botUsername string,
) (github.Reaction, bool, error) {
	signals, err := actionPendingCIRepairSignalsForPR(
		ctx, client, owner, repository, pullRequest, botUsername,
	)
	if err != nil {
		return github.Reaction{}, false, err
	}
	if signals.claim.ID != 0 {
		acquired, claimErr := consumeActionPendingCIRepairReady(
			ctx, client, owner, repository, pullRequest, signals.ready.ID,
		)

		return signals.claim, acquired, claimErr
	}
	if signals.ready.ID == 0 {
		return github.Reaction{}, false, nil
	}
	claim, err := addActionPendingCIRepairSignal(
		ctx, client, owner, repository, pullRequest,
		ReactionPendingCIRepairClaim, "claim",
	)
	if err != nil {
		return github.Reaction{}, false, err
	}
	acquired, err := consumeActionPendingCIRepairReady(
		ctx, client, owner, repository, pullRequest, signals.ready.ID,
	)

	return claim, acquired, err
}

func consumeActionPendingCIRepairReady(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	reactionID int64,
) (bool, error) {
	if reactionID == 0 {
		return true, nil
	}
	err := client.RemovePullRequestReaction(
		ctx, owner, repository, pullRequest, reactionID,
	)
	var apiErr *github.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim pending-CI repair signal: %w", err)
	}

	return true, nil
}

func signalActionPendingCIRepair(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
) error {
	_, err := addActionPendingCIRepairSignal(
		ctx, client, owner, repository, pullRequest,
		ReactionPendingCIRepair, "signal",
	)

	return err
}

func addActionPendingCIRepairSignal(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	reactionType github.ReactionType,
	subject string,
) (github.Reaction, error) {
	created, err := client.AddPullRequestReactionState(
		ctx, owner, repository, pullRequest, reactionType,
	)
	if err != nil {
		return github.Reaction{}, fmt.Errorf("record pending-CI repair %s: %w", subject, err)
	}
	if created.ID == 0 {
		return github.Reaction{},
			fmt.Errorf("GitHub returned an incomplete pending-CI repair %s", subject)
	}

	return created, nil
}

func clearActionPendingCIRepairClaim(
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
	if err != nil {
		return fmt.Errorf("clear pending-CI repair claim: %w", err)
	}

	return nil
}

func retryActionPendingCIRepair(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	cause error,
) (PendingCIPR, bool, error) {
	_, retryErr := addActionPendingCIRepairSignal(
		ctx, client, owner, repository, pullRequest,
		ReactionPendingCIRepairClaim, "claim",
	)

	return PendingCIPR{}, false, errors.Join(cause, retryErr)
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
