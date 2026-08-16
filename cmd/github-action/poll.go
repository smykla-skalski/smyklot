package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/permissions"
)

const (
	flagPollRepo  = "repo"
	flagPollToken = "token"
	descPollRepo  = "Repository in format owner/name"
	descPollToken = "GitHub API token" //nolint:gosec // Flag description, not a credential
)

var pollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll reactions on open PRs",
	Long: `Poll reactions on all open pull requests and process them.

This command fetches all open PRs, checks reactions on all comments,
and processes reaction-based commands (approve, merge, cleanup).

Designed to be run on a schedule (cron) to enable reaction support
in GitHub Actions without webhooks.`,
	RunE: runPoll,
}

func init() {
	pollCmd.Flags().StringP(flagPollRepo, "r", "", descPollRepo)
	pollCmd.Flags().StringP(flagPollToken, "t", "", descPollToken)

	// The sweep takes the same settings flags as the Action, so a workflow
	// that passes one to one command can pass it to the other
	config.RegisterFlags(pollCmd.Flags())

	rootCmd.AddCommand(pollCmd)
}

func runPoll(cmd *cobra.Command, _ []string) error {
	// Create context from command
	ctx := cmd.Context()

	// Create runtime config for GitHub App auth
	rc := &RuntimeConfig{}
	loadEnvIfEmpty(&rc.Token, envGitHubToken)
	loadEnvIfEmpty(&rc.GitHubAppPrivateKey, envGitHubAppPrivateKey)
	loadEnvIfEmpty(&rc.GitHubAppClientID, envGitHubAppClientID)
	loadEnvIfEmpty(&rc.GitHubAppID, envGitHubAppID)
	loadEnvIfEmpty(&rc.InstallationID, envInstallationID)
	loadEnvIfEmpty(&rc.BotUsername, envBotUsername)
	loadEnvIfEmpty(&rc.APIBaseURL, envAPIBaseURL)

	if rc.BotUsername == "" {
		rc.BotUsername = defaultBotUsername
	}

	// Load bot configuration
	bc, err := loadBotConfig(cmd)
	if err != nil {
		return err
	}

	// Get configuration from flags and environment
	repo, token, err := getPollConfig(cmd, rc)
	if err != nil {
		return err
	}

	// Parse repository owner and name
	repoOwner, repoName, err := parseRepo(repo)
	if err != nil {
		return err
	}

	client, err := github.NewClient(token, rc.APIBaseURL)
	if err != nil {
		return NewGitHubError(ErrGitHubClient, err)
	}

	// Layer the repository's own configuration over the workflow's, the same
	// way a comment does. A sweep that ignored the file would act on reactions
	// with settings the repository had turned off, and would keep sweeping a
	// repository that has moved to the service
	bc, err = effectiveConfig(ctx, client, repoOwner, repoName, bc)
	if err != nil {
		return reportUnusableRepoConfig(ctx, err)
	}

	// Decided before CODEOWNERS is read, the way the service's sweep decides
	// it. A repository on the service runs this workflow every five minutes,
	// and reading a file it will not use is the whole cost of that tick
	if actionStandsDown(ctx, bc) {
		return nil
	}

	checker, err := newPermissionChecker(ctx, client, repoOwner, repoName)
	if err != nil {
		return err
	}

	// Poll and process all open PRs
	return pollAllPRs(
		ctx, client, checker, bc, repoOwner, repoName, rc.BotUsername,
		commandEnvironment{}, true,
	)
}

// getPollConfig retrieves repo and token from flags or environment
func getPollConfig(cmd *cobra.Command, rc *RuntimeConfig) (string, string, error) {
	repo, err := cmd.Flags().GetString(flagPollRepo)
	if err != nil {
		return "", "", err
	}

	token, err := cmd.Flags().GetString(flagPollToken)
	if err != nil {
		return "", "", err
	}

	// Get repo from environment if not provided via flag
	if repo == "" {
		owner := os.Getenv(envRepoOwner)
		name := os.Getenv(envRepoName)
		if owner == "" || name == "" {
			return "", "", fmt.Errorf("repository not specified (use --repo or REPO_OWNER/REPO_NAME env vars)")
		}
		repo = repoFullName(owner, name)
	}

	// Get token from environment if not provided via flag
	if token == "" {
		token = os.Getenv(envGitHubToken)
		if token == "" {
			// Try GitHub App auth
			installationToken, err := getInstallationToken(rc)
			if err != nil {
				return "", "", err
			}
			if installationToken != "" {
				token = installationToken
			}
		}
	}

	if token == "" {
		return "", "", fmt.Errorf("GitHub token not specified (use --token or GITHUB_TOKEN env var)")
	}

	return repo, token, nil
}

// repoFullName is how a repository is named everywhere it is spoken about: in
// a log line, in a cache key, and in the string parseRepo reads back.
func repoFullName(owner, name string) string {
	return fmt.Sprintf("%s/%s", owner, name)
}

// parseRepo parses owner and name from repo string
func parseRepo(repo string) (string, string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository format (expected owner/name, got %q)", repo)
	}
	return parts[0], parts[1], nil
}

// newPermissionChecker builds a permission checker for a repository from a
// client that already exists.
//
// The service polls many repositories through one installation client, so it
// needs the checker without a client to go with it.
func newPermissionChecker(
	ctx context.Context,
	client *github.Client,
	repoOwner, repoName string,
) (*permissions.Checker, error) {
	codeownersContent, err := fetchCodeowners(ctx, client, repoOwner, repoName, nil)
	if err != nil {
		return nil, err
	}

	return checkerFromCodeowners(codeownersContent, client)
}

// fetchCodeowners reads a repository's CODEOWNERS, or empty when it has none.
//
// It takes what the cache already holds and ignores it: CODEOWNERS is one
// request to read, so there is nothing cheaper to ask first.
func fetchCodeowners(
	ctx context.Context,
	client *github.Client,
	repoOwner, repoName string,
	_ *string,
) (string, error) {
	content, err := client.GetCodeowners(ctx, repoOwner, repoName)
	if err != nil {
		return "", NewGitHubError(ErrGetCodeowners, err)
	}

	// Log if CODEOWNERS is missing
	if content == "" {
		logging.From(ctx).Warn("no CODEOWNERS, falling back to repository admin permissions",
			"repo", repoFullName(repoOwner, repoName))
	}

	return content, nil
}

// checkerFromCodeowners parses CODEOWNERS content into a permission checker.
//
// Kept separate from the fetch so the service can cache the content but still
// bind each checker to the client holding the current installation token.
func checkerFromCodeowners(content string, client *github.Client) (*permissions.Checker, error) {
	checker, err := permissions.NewCheckerFromContent(content, client)
	if err != nil {
		return nil, NewGitHubError(ErrInitPermissions, err)
	}

	return checker, nil
}

// pollAllPRs polls and processes reactions on all open PRs
func pollAllPRs(
	ctx context.Context,
	client *github.Client,
	checker *permissions.Checker,
	bc *config.Config,
	repoOwner, repoName string,
	botUsername string,
	environment commandEnvironment,
	includePendingCI bool,
) error {
	// Named once, here, so every line below carries the repository without
	// each of them having to say so
	ctx = logging.With(ctx, "repo", repoFullName(repoOwner, repoName))

	logging.From(ctx).Info("polling PR reactions")

	// Get all open PRs
	prs, err := client.GetOpenPRs(ctx, repoOwner, repoName)
	if err != nil {
		return NewGitHubError(ErrGetPRs, err)
	}

	return processAllPRs(
		ctx, client, checker, bc, repoOwner, repoName, botUsername, prs,
		environment, includePendingCI,
	)
}

func processAllPRs(
	ctx context.Context,
	client *github.Client,
	checker *permissions.Checker,
	bc *config.Config,
	repoOwner, repoName, botUsername string,
	prs []map[string]interface{},
	environment commandEnvironment,
	includePendingCI bool,
) error {
	if len(prs) == 0 {
		logging.From(ctx).Info("no open PRs")

		return nil
	}

	logging.From(ctx).Info("found open PRs", "count", len(prs))

	// Process reactions on each PR
	for _, pr := range prs {
		if err := processPR(
			ctx, client, checker, bc, repoOwner, repoName, pr, environment,
		); err != nil {
			logging.From(ctx).Warn("failed to process PR reactions", "error", err)
		}
	}

	// Only the legacy Action sweep discovers pending work from labels. The
	// service's fallback is the durable scheduler, so terminal requests can
	// never be resurrected by a stale label.
	if includePendingCI {
		if err := processPendingCIPRs(ctx, client, bc, repoOwner, repoName, prs, botUsername); err != nil {
			logging.From(ctx).Warn("failed to process pending-CI PRs", "error", err)
		}
	}

	logging.From(ctx).Info("polling complete")

	return nil
}

// processPR processes reactions on a single PR
func processPR(
	ctx context.Context,
	client *github.Client,
	checker *permissions.Checker,
	bc *config.Config,
	repoOwner, repoName string,
	pr map[string]interface{},
	environment commandEnvironment,
) error {
	prNumberFloat, ok := pr["number"].(float64)
	if !ok {
		return fmt.Errorf("invalid PR number")
	}
	prNumber := int(prNumberFloat)

	ctx = logging.With(ctx, "pr", prNumber)

	logging.From(ctx).Debug("processing PR")

	// Get PR author and title for RuntimeConfig
	var author, title string
	if user, ok := pr["user"].(map[string]interface{}); ok {
		if login, ok := user["login"].(string); ok {
			author = login
		}
	}
	if t, ok := pr["title"].(string); ok {
		title = t
	}

	// Create request-scoped runtime config for this PR
	rc := &RuntimeConfig{
		CommentBody:   title, // Use PR title as body
		CommentID:     strconv.Itoa(prNumber),
		CommentAction: commentActionCreated,
		PRNumber:      strconv.Itoa(prNumber),
		RepoOwner:     repoOwner,
		RepoName:      repoName,
		CommentAuthor: author,
		BotUsername:   defaultBotUsername, // Use default bot username
	}

	// Process reactions if not disabled
	if !bc.DisableReactions {
		if err := handleReactions(
			ctx, client, rc, bc, checker, prNumber, prNumber, environment,
		); err != nil {
			return fmt.Errorf("failed to process reactions on PR #%d: %w", prNumber, err)
		}
	}

	return nil
}

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
				"pr", extractPRNumber(pr.prData), "error", err)
		}
	}

	return nil
}

// pendingCIPR holds data about a PR waiting for CI
type pendingCIPR struct {
	prData       map[string]interface{}
	method       github.MergeMethod
	label        string
	requiredOnly bool // true if only required checks should be considered
}

// filterPendingCIPRs filters PRs that have pending-ci labels
func filterPendingCIPRs(prs []map[string]interface{}) []pendingCIPR {
	var result []pendingCIPR

	for _, pr := range prs {
		labels := pendingCILabels(pr)
		if len(labels) > 0 {
			result = append(result, labels[0])
		}
	}

	return result
}

func pullRequestHasLabel(pr map[string]interface{}, wanted string) bool {
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

// pendingCILabels returns every pending-CI label on one pull request. Action
// polling intentionally consumes only the first; upgrade cleanup needs all of
// them so no stale method label survives the cutover.
func pendingCILabels(pr map[string]interface{}) []pendingCIPR {
	labels, ok := pr["labels"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]pendingCIPR, 0, len(labels))
	for _, item := range labels {
		labelMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		labelName, ok := labelMap["name"].(string)
		if !ok {
			continue
		}
		method, requiredOnly, label := parsePendingCILabel(labelName)
		if label == "" {
			continue
		}
		result = append(result, pendingCIPR{
			prData: pr, method: method, label: label, requiredOnly: requiredOnly,
		})
	}

	return result
}

// parsePendingCILabel parses a pending-ci label and returns the merge method and required flag
//
// Returns:
// - MergeMethod, requiredOnly bool, and label name if valid pending-ci label
// - Empty string if not a pending-ci label
func parsePendingCILabel(label string) (github.MergeMethod, bool, string) {
	switch label {
	case github.LabelPendingCIMerge, github.LegacyLabelPendingCIMerge:
		return github.MergeMethodMerge, false, label
	case github.LabelPendingCISquash, github.LegacyLabelPendingCISquash:
		return github.MergeMethodSquash, false, label
	case github.LabelPendingCIRebase, github.LegacyLabelPendingCIRebase:
		return github.MergeMethodRebase, false, label
	case github.LabelPendingCIMergeRequired, github.LegacyLabelPendingCIMergeRequired:
		return github.MergeMethodMerge, true, label
	case github.LabelPendingCISquashRequired, github.LegacyLabelPendingCISquashRequired:
		return github.MergeMethodSquash, true, label
	case github.LabelPendingCIRebaseRequired, github.LegacyLabelPendingCIRebaseRequired:
		return github.MergeMethodRebase, true, label
	default:
		return "", false, ""
	}
}

// extractPRNumber extracts PR number from PR data
func extractPRNumber(pr map[string]interface{}) int {
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
	pr pendingCIPR,
	botUsername string,
) error {
	prNumber := extractPRNumber(pr.prData)
	if prNumber == 0 {
		return fmt.Errorf("invalid PR number")
	}

	ctx = logging.With(ctx, "pr", prNumber)

	logging.From(ctx).Debug("checking CI status", "merge_method", pr.method)

	// Get PR head SHA for CI status check
	headRef, err := client.GetPRHeadRef(ctx, repoOwner, repoName, prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR head ref: %w", err)
	}
	actionOwned, err := pendingCIActionOwns(
		ctx, client, repoOwner, repoName, prNumber, pr.label, headRef, botUsername,
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
	if pr.requiredOnly {
		// Get base branch from PR info
		info, err := client.GetPRInfo(ctx, repoOwner, repoName, prNumber)
		if err != nil {
			return fmt.Errorf("failed to get PR info: %w", err)
		}

		if info.BaseBranch == "" {
			return fmt.Errorf("cannot resolve base branch for required-check wait")
		}
		requiredChecks, err = client.GetRequiredStatusChecks(ctx, repoOwner, repoName, info.BaseBranch)
		if err != nil {
			return fmt.Errorf("failed to get required checks: %w", err)
		}
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
	pr pendingCIPR,
	botUsername string,
	headRef string,
) error {
	logging.From(ctx).Info("CI passed, merging")
	actionOwned, err := pendingCIActionOwns(
		ctx, client, repoOwner, repoName, prNumber, pr.label, headRef, botUsername,
	)
	if err != nil {
		return err
	}
	if !actionOwned {
		logging.From(ctx).Info("pending CI ownership changed; Action stands down")

		return nil
	}

	if err := mergePendingPRAtHead(ctx, client, repoOwner, repoName, prNumber, pr.method, headRef); err != nil {
		if mergeHeadChanged(err) {
			return nil
		}

		return postPendingCIError(ctx, client, repoOwner, repoName, prNumber, pr.label, err.Error())
	}

	// Remove pending-ci label
	_ = client.RemoveLabel(ctx, repoOwner, repoName, prNumber, pr.label)

	// Update pending CI reaction from 👀 to 👍
	_ = client.UpdatePendingCIReaction(ctx, repoOwner, repoName, prNumber, botUsername)

	// Post success feedback
	// We don't know who requested the merge, so use a generic message
	fb := feedback.NewPendingCIMerged("automation", bc.QuietSuccess)
	if fb.RequiresComment() {
		_ = client.PostComment(ctx, repoOwner, repoName, prNumber, fb.Message)
	}

	return nil
}

func mergePendingPRAtHead(
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
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return false
	}

	return true
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
