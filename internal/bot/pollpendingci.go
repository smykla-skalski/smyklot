package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
)

// processPendingCIPRs processes PRs that are waiting for CI to pass before merge
//
// It queries PRs with pending-ci labels, checks their CI status, and:
// - Merges if CI passes
// - Removes label and posts failure feedback if CI fails
// - Skips if CI is still pending
//
//nolint:unparam // error return kept for consistent function signature and future error handling
func processPendingCIPRs(
	ctx context.Context,
	client *github.Client,
	bc *config.Config,
	repoOwner, repoName string,
	prs []map[string]interface{},
	botUsername string,
) error {
	pendingPRs := filterPendingCIPRs(prs)
	for _, recovered := range recoverActionPendingCIRepairs(
		ctx, client, bc, repoOwner, repoName, prs, botUsername,
	) {
		pendingPRs = appendPendingCIRequest(pendingPRs, recovered)
	}

	if len(pendingPRs) == 0 {
		return nil
	}

	logging.From(ctx).Info("processing PRs waiting for CI", "count", len(pendingPRs))

	for _, pr := range pendingPRs {
		if err := processPendingCIPR(ctx, client, bc, repoOwner, repoName, pr, botUsername); err != nil {
			logging.From(ctx).Warn("failed to process pending-CI PR",
				"pr", ExtractPRNumber(pr.PRData), "error", err)
		}
	}

	return nil
}

// PendingCIPR holds data about a PR waiting for CI
type PendingCIPR struct {
	PRData       map[string]interface{}
	Method       github.MergeMethod
	Label        string
	RequiredOnly bool // true if only required checks should be considered
}

// filterPendingCIPRs filters PRs that have pending-ci labels
func filterPendingCIPRs(prs []map[string]interface{}) []PendingCIPR {
	var result []PendingCIPR

	for _, pr := range prs {
		labels := PendingCILabels(pr)
		if len(labels) > 0 {
			result = append(result, labels[0])
		}
	}

	return result
}

func PullRequestHasLabel(pr map[string]interface{}, wanted string) bool {
	labels, ok := pr["labels"].([]interface{})
	if !ok {
		return false
	}
	for _, item := range labels {
		label, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := label["name"].(string); name == wanted {
			return true
		}
	}

	return false
}

// PendingCILabels returns every pending-CI label on one pull request. Action
// polling intentionally consumes only the first; upgrade cleanup needs all of
// them so no stale method label survives the cutover.
func PendingCILabels(pr map[string]interface{}) []PendingCIPR {
	labels, ok := pr["labels"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]PendingCIPR, 0, len(labels))
	for _, item := range labels {
		labelMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		labelName, ok := labelMap["name"].(string)
		if !ok {
			continue
		}
		method, requiredOnly, label := ParsePendingCILabel(labelName)
		if label == "" {
			continue
		}
		result = append(result, PendingCIPR{
			PRData: pr, Method: method, Label: label, RequiredOnly: requiredOnly,
		})
	}

	return result
}

// ExtractPRNumber extracts PR number from PR data
func ExtractPRNumber(pr map[string]interface{}) int {
	if num, ok := pr["number"].(float64); ok {
		return int(num)
	}

	return 0
}

// processPendingCIPR processes a single PR waiting for CI
func processPendingCIPR(
	ctx context.Context,
	client *github.Client,
	bc *config.Config,
	repoOwner, repoName string,
	pr PendingCIPR,
	botUsername string,
) error {
	prNumber := ExtractPRNumber(pr.PRData)
	if prNumber == 0 {
		return fmt.Errorf("invalid PR number")
	}

	ctx = logging.With(ctx, "pr", prNumber)

	logging.From(ctx).Debug("checking CI status", "merge_method", pr.Method)

	state, err := client.GetPullRequestState(ctx, repoOwner, repoName, prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR state: %w", err)
	}
	headRef := state.HeadSHA
	actionOwned, err := pendingCIActionOwns(
		ctx, client, repoOwner, repoName, prNumber, pr.Label, headRef, botUsername,
	)
	if err != nil {
		return err
	}
	if !actionOwned {
		logging.From(ctx).Info("pending CI request is owned by the service; Action stands down")

		return nil
	}
	cancelled, err := cancelActionPendingCIDraft(
		ctx, client, bc, repoOwner, repoName, prNumber, pr,
		botUsername, state.Draft,
	)
	if err != nil || cancelled {
		return err
	}

	// Get required checks list if filtering by required checks only
	var requiredChecks []github.RequiredCheck
	if pr.RequiredOnly {
		// Get base branch from PR info
		info, err := client.GetPRInfo(ctx, repoOwner, repoName, prNumber)
		if err != nil {
			return fmt.Errorf("failed to get PR info: %w", err)
		}

		if info.BaseBranch == "" {
			return fmt.Errorf("cannot resolve base branch for required-check wait")
		}
		requirements, requirementsErr := client.GetRequiredCIRequirements(
			ctx, repoOwner, repoName, info.BaseBranch,
		)
		err = requirementsErr
		if err != nil {
			return fmt.Errorf("failed to get required checks: %w", err)
		}
		if requirements.RequiredWorkflow {
			return ErrRequiredWorkflowsUnsupported
		}
		requiredChecks = requirements.StatusChecks
		if len(requiredChecks) == 0 {
			return fmt.Errorf("base branch has no required status checks")
		}
	}

	// Check current CI status
	checkStatus, err := client.GetCheckStatus(ctx, repoOwner, repoName, headRef, requiredChecks)
	if err != nil {
		return fmt.Errorf("failed to get CI status: %w", err)
	}

	// Handle based on CI status
	if checkStatus.State == github.CIStatePassing {
		return handlePendingCIPassed(
			ctx,
			client,
			bc,
			repoOwner,
			repoName,
			prNumber,
			pr,
			botUsername,
			headRef,
		)
	}

	// A red, missing, or indeterminate observation is not terminal. A rerun or
	// newly-created check can still make this same request eligible to merge.
	logging.From(ctx).Debug("CI wait remains armed", "state", checkStatus.State, "summary", checkStatus.Summary)

	return nil
}

func cancelActionPendingCIDraft(
	ctx context.Context,
	client *github.Client,
	bc *config.Config,
	owner, repository string,
	pullRequest int,
	pr PendingCIPR,
	botUsername string,
	draft bool,
) (bool, error) {
	cancelled, err := actionPendingCIDraftCancelled(
		ctx, client, bc, owner, repository, pullRequest,
		pr.Method, pr.RequiredOnly, pr.Label, botUsername, draft,
	)
	if err != nil {
		return false, fmt.Errorf("verify pending CI draft history: %w", err)
	}
	if !cancelled {
		return false, nil
	}

	return cancelDraftPendingCI(
		ctx, client, bc, owner, repository, pullRequest, pr, botUsername,
	)
}

// handlePendingCIPassed handles a PR where CI has passed
func handlePendingCIPassed(
	ctx context.Context,
	client *github.Client,
	bc *config.Config,
	repoOwner, repoName string,
	prNumber int,
	pr PendingCIPR,
	botUsername string,
	headRef string,
) error {
	logging.From(ctx).Info("CI passed, merging")
	actionOwned, err := pendingCIActionOwns(
		ctx, client, repoOwner, repoName, prNumber, pr.Label, headRef, botUsername,
	)
	if err != nil {
		return err
	}
	if !actionOwned {
		logging.From(ctx).Info("pending CI ownership changed; Action stands down")

		return nil
	}
	authorize := func() error {
		state, stateErr := client.GetPullRequestState(ctx, repoOwner, repoName, prNumber)
		if stateErr != nil {
			return fmt.Errorf("revalidate pending CI draft state: %w", stateErr)
		}
		cancelled, cancelErr := cancelActionPendingCIDraft(
			ctx, client, bc, repoOwner, repoName, prNumber, pr,
			botUsername, state.Draft,
		)
		if cancelErr != nil {
			return cancelErr
		}
		if cancelled {
			return pendingci.ErrStaleSourceRevision
		}

		return nil
	}
	if err := MergePendingPRAtHead(
		ctx, client, repoOwner, repoName, prNumber, pr.Method, headRef, authorize,
	); err != nil {
		if errors.Is(err, pendingci.ErrStaleSourceRevision) ||
			errors.Is(err, pendingci.ErrAmbiguousSourceRevision) {
			return nil
		}
		if mergeHeadChanged(err) {
			return nil
		}

		return postPendingCIError(ctx, client, repoOwner, repoName, prNumber, pr.Label, err.Error())
	}

	// Remove pending-ci label
	_ = client.RemoveLabel(ctx, repoOwner, repoName, prNumber, pr.Label)

	// Update pending CI reaction from 👀 to 👍
	_ = settlePendingCIReaction(
		ctx, client, repoOwner, repoName, prNumber, botUsername, ReactionSuccess,
	)

	// Post success feedback
	// We don't know who requested the merge, so use a generic message
	fb := feedback.NewPendingCIMerged("automation", bc.QuietSuccess)
	if fb.RequiresComment() {
		_ = client.PostComment(ctx, repoOwner, repoName, prNumber, fb.Message)
	}

	return nil
}

func MergePendingPRAtHead(
	ctx context.Context,
	client *github.Client,
	owner, repo string,
	prNumber int,
	method github.MergeMethod,
	headRef string,
	authorize func() error,
) error {
	if err := authorize(); err != nil {
		return err
	}
	err := client.MergePRAtHead(ctx, owner, repo, prNumber, method, headRef)
	if err == nil || method != github.MergeMethodMerge || !strings.Contains(err.Error(), "Merge commits are not allowed") {
		return err
	}

	if err = authorize(); err != nil {
		return err
	}
	err = client.MergePRAtHead(ctx, owner, repo, prNumber, github.MergeMethodSquash, headRef)
	if err == nil || mergeHeadChanged(err) {
		return err
	}

	if err = authorize(); err != nil {
		return err
	}

	return client.MergePRAtHead(ctx, owner, repo, prNumber, github.MergeMethodRebase, headRef)
}

func mergeHeadChanged(err error) bool {
	var apiErr *github.APIError

	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}

// settlePendingCIReaction finds comments with the bot's "eyes" reaction and replaces with "+1"
//
// This is used after a pending-ci merge succeeds to update the visual feedback.
// It searches all comments on the PR, finds ones with "eyes" reaction from the bot,
// removes the "eyes" reaction, and adds a "+1" (thumbs up) reaction.
func settlePendingCIReaction(
	ctx context.Context,
	client *github.Client,
	owner, repo string,
	prNumber int,
	botUsername string,
	result github.ReactionType,
) error {
	// Get all comments on the PR
	comments, err := client.GetPRComments(ctx, owner, repo, prNumber)
	if err != nil {
		return err
	}
	var cleanupErrors []error

	// Check each comment for bot's "eyes" reaction
	for _, comment := range comments {
		if err := settlePendingCIComment(
			ctx, client, owner, repo, int(comment.ID), botUsername, result,
		); err != nil {
			cleanupErrors = append(
				cleanupErrors,
				fmt.Errorf("settle pending CI reaction on comment %d: %w", comment.ID, err),
			)
		}
	}

	return errors.Join(cleanupErrors...)
}

func settlePendingCIComment(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	commentID int,
	botUsername string,
	result github.ReactionType,
) error {
	reactions, err := client.GetCommentReactions(ctx, owner, repository, commentID)
	if err != nil {
		return err
	}
	pending := false
	for _, reaction := range reactions {
		if reaction.User == botUsername &&
			(reaction.Type == ReactionPendingCI || reaction.Type == ReactionPendingCIAction) {
			pending = true

			break
		}
	}
	if !pending {
		return nil
	}
	if err := errors.Join(
		client.RemoveReactionByUser(
			ctx, owner, repository, commentID, ReactionPendingCI, botUsername,
		),
		client.RemoveReactionByUser(
			ctx, owner, repository, commentID, ReactionPendingCIAction, botUsername,
		),
	); err != nil {
		return err
	}

	return client.AddReaction(ctx, owner, repository, commentID, result)
}

func cancelDraftPendingCI(
	ctx context.Context,
	client *github.Client,
	bc *config.Config,
	owner, repository string,
	pullRequest int,
	pr PendingCIPR,
	botUsername string,
) (bool, error) {
	return reconcileDraftPendingCI(
		ctx, client, bc, owner, repository, pullRequest, pr, botUsername, true,
	)
}

func reconcileDraftPendingCI(
	ctx context.Context,
	client *github.Client,
	bc *config.Config,
	owner, repository string,
	pullRequest int,
	pr PendingCIPR,
	botUsername string,
	notify bool,
) (bool, error) {
	state, err := client.GetPullRequestState(ctx, owner, repository, pullRequest)
	if err != nil {
		return false, fmt.Errorf("preflight cancelled pending CI state: %w", err)
	}
	stillCancelled, err := actionPendingCIDraftCancelled(
		ctx, client, bc, owner, repository, pullRequest,
		pr.Method, pr.RequiredOnly, pr.Label, botUsername, state.Draft,
	)
	if err != nil {
		return false, fmt.Errorf("preflight cancelled pending CI authorization: %w", err)
	}
	if !stillCancelled {
		return false, nil
	}
	labelErr := client.RemoveLabel(ctx, owner, repository, pullRequest, pr.Label)
	if labelErr != nil {
		return false, fmt.Errorf("remove cancelled pending CI label: %w", labelErr)
	}
	state, err = client.GetPullRequestState(ctx, owner, repository, pullRequest)
	if err != nil {
		return false, repairActionPendingCILabel(
			ctx, client, bc, owner, repository, pullRequest,
			pr.Method, pr.RequiredOnly, botUsername, pr.Label,
			actionPendingCIArtifactExclusion{},
			fmt.Errorf("recheck cancelled pending CI state: %w", err),
		)
	}
	stillCancelled, err = actionPendingCIDraftCancelled(
		ctx, client, bc, owner, repository, pullRequest,
		pr.Method, pr.RequiredOnly, pr.Label, botUsername, state.Draft,
	)
	if err != nil {
		return false, repairActionPendingCILabel(
			ctx, client, bc, owner, repository, pullRequest,
			pr.Method, pr.RequiredOnly, botUsername, pr.Label,
			actionPendingCIArtifactExclusion{},
			fmt.Errorf("recheck cancelled pending CI authorization: %w", err),
		)
	}
	if !stillCancelled {
		return restorePendingCILabel(
			ctx, client, owner, repository, pullRequest, pr.Label, nil,
		)
	}
	surviving, cleanupErr := settleCancelledActionPendingCI(
		ctx, client, bc, owner, repository, pullRequest,
		pr.Method, pr.RequiredOnly, botUsername, pr.Label, state.Draft,
	)
	if surviving || cleanupErr != nil {
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("clean up cancelled pending CI request: %w", cleanupErr)
		}

		return false, repairActionPendingCILabel(
			ctx, client, bc, owner, repository, pullRequest,
			pr.Method, pr.RequiredOnly, botUsername, pr.Label,
			actionPendingCIArtifactExclusion{}, cleanupErr,
		)
	}
	if !notify {
		return true, nil
	}
	fb := feedback.NewPendingCICancelled(pendingci.DraftCancellationReason)

	return true, client.PostComment(ctx, owner, repository, pullRequest, fb.Message)
}

func repairActionPendingCILabel(
	ctx context.Context,
	client *github.Client,
	botConfig *config.Config,
	owner, repository string,
	pullRequest int,
	method github.MergeMethod,
	requiredOnly bool,
	botUsername string,
	label string,
	exclusion actionPendingCIArtifactExclusion,
	cause error,
) error {
	disarmErr := client.RemoveLabel(ctx, owner, repository, pullRequest, label)
	var apiErr *github.APIError
	if errors.As(disarmErr, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		disarmErr = nil
	}
	artifacts, _, scanErr := actionPendingCIArtifacts(
		ctx, client, botConfig, owner, repository, pullRequest,
		method, requiredOnly, botUsername, exclusion,
	)
	if scanErr != nil {
		// A PR reaction keeps the repair discoverable without recreating a
		// method label, which is an authorization artifact for legacy waits.
		retryErr := signalActionPendingCIRepair(
			ctx, client, owner, repository, pullRequest,
		)

		return errors.Join(cause, disarmErr, scanErr, retryErr)
	}
	for _, artifact := range artifacts {
		if artifact.legacy || !artifact.bound {
			continue
		}
		authorizationErr := ValidateDraftMergeAuthorization(
			ctx, client, owner, repository, pullRequest, artifact.revision,
		)
		if errors.Is(authorizationErr, pendingci.ErrStaleSourceRevision) ||
			errors.Is(authorizationErr, pendingci.ErrAmbiguousSourceRevision) {
			continue
		}
		if authorizationErr != nil {
			retryErr := signalActionPendingCIRepair(
				ctx, client, owner, repository, pullRequest,
			)

			return errors.Join(cause, disarmErr, authorizationErr, retryErr)
		}
		restoreErr := client.AddLabel(ctx, owner, repository, pullRequest, label)
		if restoreErr != nil {
			retryErr := signalActionPendingCIRepair(
				ctx, client, owner, repository, pullRequest,
			)

			return errors.Join(cause, disarmErr, restoreErr, retryErr)
		}

		return errors.Join(cause, disarmErr)
	}

	return errors.Join(cause, disarmErr)
}

func restorePendingCILabel(
	ctx context.Context,
	client *github.Client,
	owner, repository string,
	pullRequest int,
	label string,
	cause error,
) (bool, error) {
	restoreErr := client.AddLabel(ctx, owner, repository, pullRequest, label)
	if restoreErr != nil {
		restoreErr = fmt.Errorf("restore concurrent pending CI command: %w", restoreErr)
		retryErr := signalActionPendingCIRepair(
			ctx, client, owner, repository, pullRequest,
		)

		return false, errors.Join(cause, restoreErr, retryErr)
	}

	return false, cause
}

// postPendingCIError posts error feedback and removes a request that cannot be completed.
func postPendingCIError(
	ctx context.Context,
	client *github.Client,
	repoOwner, repoName string,
	prNumber int,
	label, reason string,
) error {
	// Remove pending-ci label
	_ = client.RemoveLabel(ctx, repoOwner, repoName, prNumber, label)

	// Post failure feedback
	fb := feedback.NewPendingCIFailed(reason)
	_ = client.PostComment(ctx, repoOwner, repoName, prNumber, fb.Message)

	return nil
}
