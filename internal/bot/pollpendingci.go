package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	// Filter PRs with pending-ci labels
	pendingPRs := filterPendingCIPRs(prs)

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

	// Get PR head SHA for CI status check
	headRef, err := client.GetPRHeadRef(ctx, repoOwner, repoName, prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR head ref: %w", err)
	}
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

	if err := MergePendingPRAtHead(ctx, client, repoOwner, repoName, prNumber, pr.Method, headRef); err != nil {
		if mergeHeadChanged(err) {
			return nil
		}

		return postPendingCIError(ctx, client, repoOwner, repoName, prNumber, pr.Label, err.Error())
	}

	// Remove pending-ci label
	_ = client.RemoveLabel(ctx, repoOwner, repoName, prNumber, pr.Label)

	// Update pending CI reaction from 👀 to 👍
	_ = settlePendingCIReaction(ctx, client, repoOwner, repoName, prNumber, botUsername)

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
) error {
	err := client.MergePRAtHead(ctx, owner, repo, prNumber, method, headRef)
	if err == nil || method != github.MergeMethodMerge || !strings.Contains(err.Error(), "Merge commits are not allowed") {
		return err
	}

	err = client.MergePRAtHead(ctx, owner, repo, prNumber, github.MergeMethodSquash, headRef)
	if err == nil || mergeHeadChanged(err) {
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
) error {
	// Get all comments on the PR
	comments, err := client.GetPRComments(ctx, owner, repo, prNumber)
	if err != nil {
		return err
	}

	// Check each comment for bot's "eyes" reaction
	for _, comment := range comments {
		commentID := int(comment.ID)

		// Get reactions for this comment
		reactions, err := client.GetCommentReactions(ctx, owner, repo, commentID)
		if err != nil {
			continue // Skip comments we can't get reactions for
		}

		// Check if bot has an "eyes" reaction on this comment
		hasBotEyesReaction := false

		for _, reaction := range reactions {
			if reaction.User == botUsername && reaction.Type == ReactionPendingCI {
				hasBotEyesReaction = true

				break
			}
		}

		if hasBotEyesReaction {
			// Remove the "eyes" reaction
			_ = client.RemoveReactionByUser(
				ctx, owner, repo, commentID, ReactionPendingCI, botUsername,
			)

			// Add "+1" (thumbs up) reaction
			_ = client.AddReaction(ctx, owner, repo, commentID, ReactionSuccess)
		}
	}

	return nil
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
