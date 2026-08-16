// Package main provides the GitHub Actions entrypoint for Smyklot bot.
//
// Smyklot automates PR approvals and merges based on CODEOWNERS permissions.
// It reads environment variables from GitHub Actions and executes commands
// (/approve, @smyklot approve, approve, lgtm, merge, unapprove, help) based on
// user permissions defined in the .github/CODEOWNERS file.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/pkg/commands"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/permissions"
)

const (
	envGitHubToken         = "GITHUB_TOKEN" //nolint:gosec // Environment variable name, not a credential
	envCommentBody         = "COMMENT_BODY"
	envCommentID           = "COMMENT_ID"
	envCommentAction       = "COMMENT_ACTION"
	envPRNumber            = "PR_NUMBER"
	envRepoOwner           = "REPO_OWNER"
	envRepoName            = "REPO_NAME"
	envCommentAuthor       = "COMMENT_AUTHOR"
	envGitHubAppPrivateKey = "GITHUB_APP_PRIVATE_KEY" //nolint:gosec // Environment variable name, not a credential
	envGitHubAppClientID   = "GITHUB_APP_CLIENT_ID"   //nolint:gosec // Environment variable name, not a credential
	envGitHubAppID         = "GITHUB_APP_ID"          //nolint:gosec // Environment variable name, not a credential
	envInstallationID      = "GITHUB_INSTALLATION_ID"
	envBotUsername         = "SMYKLOT_BOT_USERNAME"
	envAPIBaseURL          = "SMYKLOT_GITHUB_API_URL"
	envStepSummary         = "GITHUB_STEP_SUMMARY"
	commentActionCreated   = "created"
	commentActionDeleted   = "deleted"
	rootPath               = "/"
	summaryTemplateName    = "summary"
	appName                = "smyklot"      // The binary's own name, in usage output
	defaultBotUsername     = "smyklot[bot]" // Default GitHub App bot username
	flagToken              = "token"
	flagCommentBody        = "comment-body"
	flagCommentID          = "comment-id"
	flagPRNumber           = "pr-number"
	flagRepoOwner          = "repo-owner"
	flagRepoName           = "repo-name"
	flagCommentAuthor      = "comment-author"
	descToken              = "GitHub API token" //nolint:gosec // Flag description, not a credential
	descCommentBody        = "PR comment body"
	descCommentID          = "PR comment ID"
	descPRNumber           = "Pull request number"
	descRepoOwner          = "Repository owner"
	descRepoName           = "Repository name"
	descCommentAuthor      = "Comment author username"
	errInvalidPRNum        = "invalid PR number"
	errInvalidComment      = "invalid comment ID"
	errInvalidInstallID    = "invalid installation ID"
	errCommentTooLong      = "comment body exceeds maximum length"
	errInvalidRepoName     = "invalid repository owner or name"
	selfApprovalNotAllowed = "(self-approval not allowed)"
	maxCommentBodyLength   = 10000 // 10KB - cap on untrusted comment bodies
	stepSummaryTemplate    = `## Smyklot Configuration

### Runtime Configuration

| Parameter | Value |
|-----------|-------|
| Repository | ` + "`{{.RepoOwner}}/{{.RepoName}}`" + ` |
| PR Number | ` + "`#{{.PRNumber}}`" + ` |
| Comment ID | ` + "`{{.CommentID}}`" + ` |
| Author | ` + "`@{{.CommentAuthor}}`" + ` |
| Comment | ` + "`{{.CommentBody}}`" + ` |
{{if .GitHubApp}}| Authentication | GitHub App |
| App ID | ` + "`{{.AppID}}`" + ` |
| Installation ID | ` + "`{{.InstallationID}}`" + ` |
{{else}}| Authentication | GITHUB_TOKEN |
{{end}}
### Bot Configuration

| Setting | Value |
|---------|-------|
| Quiet Success | ` + "`{{.QuietSuccess}}`" + ` |
| Quiet Reactions | ` + "`{{.QuietReactions}}`" + ` |
| Command Prefix | ` + "`{{.CommandPrefix}}`" + ` |
| Disable Mentions | ` + "`{{.DisableMentions}}`" + ` |
| Disable Bare Commands | ` + "`{{.DisableBareCommands}}`" + ` |
| Disable Unapprove | ` + "`{{.DisableUnapprove}}`" + ` |
| Disable Reactions | ` + "`{{.DisableReactions}}`" + ` |
| Disable Deleted Comments | ` + "`{{.DisableDeletedComments}}`" + ` |
| Allow Self Approval | ` + "`{{.AllowSelfApproval}}`" + ` |
{{if .AllowedCommands}}| Allowed Commands | ` + "`{{.AllowedCommands}}`" + ` |
{{else}}| Allowed Commands | All commands allowed |
{{end}}
{{if .CommandAliases}}
### Command Aliases

| Alias | Command |
|-------|----------|
{{range $alias, $cmd := .CommandAliases}}| ` + "`{{$alias}}`" + ` | ` + "`{{$cmd}}`" + ` |
{{end}}{{end}}`
)

// RuntimeConfig holds the runtime configuration for the action
type RuntimeConfig struct {
	Token               string
	CommentBody         string
	CommentID           string
	CommentAction       string
	PRNumber            string
	RepoOwner           string
	RepoName            string
	CommentAuthor       string
	GitHubAppPrivateKey string
	GitHubAppClientID   string
	GitHubAppID         string
	InstallationID      string
	BotUsername         string // Bot username for identifying bot's own comments/reviews
	APIBaseURL          string // GitHub API base URL; empty uses the public API
}

// stepSummaryData holds data for the step summary template.
type stepSummaryData struct {
	RepoOwner              string
	RepoName               string
	PRNumber               string
	CommentID              string
	CommentAuthor          string
	CommentBody            string
	GitHubApp              bool
	AppID                  string
	InstallationID         string
	QuietSuccess           bool
	QuietReactions         bool
	CommandPrefix          string
	DisableMentions        bool
	DisableBareCommands    bool
	DisableUnapprove       bool
	DisableReactions       bool
	DisableDeletedComments bool
	AllowSelfApproval      bool
	AllowedCommands        string
	CommandAliases         map[string]string
}

// githubNamePattern validates GitHub repository and owner names
// Allows: alphanumeric, hyphens, underscores, dots (e.g., .dotfiles, foo_bar, foo-bar)
var githubNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

var rootCmd = &cobra.Command{
	Use:   appName,
	Short: "GitHub Actions bot for automated PR approvals and merges",
	Long: `Smyklot is a GitHub Actions bot that enables automated PR approvals
and merges based on CODEOWNERS permissions.

It reads environment variables from GitHub Actions and executes
commands (/approve, @smyklot approve, approve, lgtm, merge) based
on user permissions.`,
	RunE: run,
}

func init() {
	registerRunFlags(rootCmd)
}

// registerRunFlags defines run()'s flags, split out of init so tests can build
// an equivalent command
func registerRunFlags(cmd *cobra.Command) {
	// Define CLI flags for runtime configuration
	cmd.Flags().String(flagToken, "", descToken)
	cmd.Flags().String(flagCommentBody, "", descCommentBody)
	cmd.Flags().String(flagCommentID, "", descCommentID)
	cmd.Flags().String(flagPRNumber, "", descPRNumber)
	cmd.Flags().String(flagRepoOwner, "", descRepoOwner)
	cmd.Flags().String(flagRepoName, "", descRepoName)
	cmd.Flags().String(flagCommentAuthor, "", descCommentAuthor)

	// Every setting that takes a flag, defined from the one description of
	// them. This was written out by hand and had fallen behind: quiet_pending
	// had no flag at all.
	config.RegisterFlags(cmd.Flags())
}

func main() {
	// The Action and the poll sweep write into a workflow log a person reads,
	// so plain text is the default. The service overrides it with JSON, which
	// it carries on the context rather than setting here
	slog.SetDefault(logging.New(os.Stdout, logging.FormatText, slog.LevelInfo, nil))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, _ []string) error {
	// Create context from command
	ctx := cmd.Context()

	// Load runtime configuration from flags and environment
	rc := loadRuntimeConfig(cmd)

	// Every layer below the repository, in one order stated in one place
	bc, err := loadBotConfig(cmd)
	if err != nil {
		return err
	}

	// Validate required configuration
	if err := validateConfig(rc); err != nil {
		return err
	}

	// Write a step summary with effective configuration
	if err := writeStepSummary(rc, bc); err != nil {
		// Don't fail if we can't write a summary, just log and continue
		logging.From(ctx).Warn("failed to write step summary", "error", err)
	}

	// Get GitHub App installation token if configured
	token := rc.Token
	installationToken, err := getInstallationToken(rc)
	if err != nil {
		return err
	}

	if installationToken != "" {
		token = installationToken
	}

	// Create a GitHub client
	client, err := github.NewClient(token, rc.APIBaseURL)
	if err != nil {
		return NewGitHubError(ErrGitHubClient, err)
	}

	// Layer the repository's own configuration over the workflow's
	//
	// The service reads the same file, so a repository that checks one in gets
	// the same treatment whichever entry point handles the comment
	base := bc

	bc, err = effectiveConfig(ctx, client, rc.RepoOwner, rc.RepoName, base)
	if err != nil {
		if errors.Is(err, ErrRepoConfigInvalid) {
			return reportInvalidRepoConfig(ctx, client, rc, base, err)
		}

		return err
	}

	// The service handles this repository unless the file says otherwise, and
	// two entry points acting on one comment is what that setting exists to
	// stop
	if actionStandsDown(ctx, bc) {
		return nil
	}

	return executeComment(ctx, client, rc, bc)
}

// executeComment runs whatever a comment asks for, whichever entry point
// delivered it.
//
// The Action and the webhook service differ only in how they arrive here: one
// reads environment variables a workflow set, the other a signed delivery. From
// this point the two are the same code, which is what keeps their results the
// same for the same comment.
func executeComment(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
) error {
	return executeCommentWithEnvironment(ctx, client, rc, bc, commandEnvironment{})
}

func executeCommentWithEnvironment(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	environment commandEnvironment,
) error {
	// Parse the command from the comment
	//
	// A parse error still yields the commands the comment asked for, which the
	// deleted-comment branch below reports on
	parsedCmd, parseErr := commands.ParseCommand(rc.CommentBody, bc)

	// Handle deleted comments
	//
	// A deleted comment is never executed, so this branch always returns
	if rc.CommentAction == commentActionDeleted {
		deletedCommands := deletedCommandsToReport(bc, parsedCmd)
		if len(deletedCommands) == 0 {
			return nil
		}

		return handleDeletedComment(ctx, client, rc, deletedCommands)
	}

	if parseErr != nil {
		// Not a valid command, ignore silently
		return nil
	}

	// If no valid command was detected and reactions are disabled, exit early
	if !parsedCmd.IsValid && bc.DisableReactions {
		return nil
	}

	// Convert string IDs to integers
	prNum, err := strconv.Atoi(rc.PRNumber)
	if err != nil {
		return NewInputError(ErrInvalidInput, rc.PRNumber, errInvalidPRNum)
	}

	commentIDNum, err := strconv.Atoi(rc.CommentID)
	if err != nil {
		return NewInputError(ErrInvalidInput, rc.CommentID, errInvalidComment)
	}

	// Clean up any previous error reactions (in case comment was edited)
	_ = client.RemoveReactionByUser(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentIDNum,
		github.ReactionError,
		rc.BotUsername,
	)

	// Fetch CODEOWNERS content from GitHub API
	codeownersContent, err := client.GetCodeowners(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
	)
	if err != nil {
		return NewGitHubError(ErrGetCodeowners, err)
	}

	// Initialize permission checker from content
	checker, err := permissions.NewCheckerFromContent(codeownersContent, client)
	if err != nil {
		return NewGitHubError(ErrInitPermissions, err)
	}

	// Handle help command immediately (no permission check needed)
	for _, cmdType := range parsedCmd.Commands {
		if cmdType == commands.CommandHelp {
			return handleHelp(ctx, client, rc, prNum, commentIDNum)
		}
	}

	// Handle reaction-based approvals/merges if enabled
	// Only process reactions if no command was found in the comment
	if !bc.DisableReactions && !parsedCmd.IsValid {
		if err := handleReactions(
			ctx, client, rc, bc, checker, prNum, commentIDNum, environment,
		); err != nil {
			return err
		}
		// Reactions have been processed, exit early
		return nil
	}

	// No valid command found and either reactions are disabled or we already processed them
	if !parsedCmd.IsValid {
		return nil
	}

	// Check if the user has permission to execute this command
	canApprove, err := checkUserPermission(
		ctx,
		client,
		checker,
		rc.CommentAuthor,
		rc.RepoOwner,
		rc.RepoName,
		rootPath,
	)
	if err != nil {
		return NewGitHubError(ErrPermissionCheck, err)
	}

	// Handle unauthorized users
	if !canApprove {
		return handleUnauthorized(ctx, client, rc, checker, prNum, commentIDNum)
	}

	// Execute all commands and collect feedback
	var feedbacks []*feedback.Feedback
	isNewComment := rc.CommentAction == commentActionCreated || rc.CommentAction == ""

	for _, cmdType := range parsedCmd.Commands {
		var fb *feedback.Feedback
		var err error

		switch cmdType {
		case commands.CommandApprove:
			fb, err = executeApprove(ctx, client, rc, bc, prNum)
		case commands.CommandMerge:
			fb, err = executeMerge(ctx, client, rc, bc, prNum, commentIDNum, github.MergeMethodMerge, parsedCmd.WaitForCI, parsedCmd.RequiredChecksOnly, environment)
		case commands.CommandSquash:
			fb, err = executeMerge(ctx, client, rc, bc, prNum, commentIDNum, github.MergeMethodSquash, parsedCmd.WaitForCI, parsedCmd.RequiredChecksOnly, environment)
		case commands.CommandRebase:
			fb, err = executeMerge(ctx, client, rc, bc, prNum, commentIDNum, github.MergeMethodRebase, parsedCmd.WaitForCI, parsedCmd.RequiredChecksOnly, environment)
		case commands.CommandUnapprove:
			fb, err = executeUnapprove(ctx, client, rc, bc, prNum)
		case commands.CommandCleanup:
			fb, _, err = executeCoordinatedCleanup(
				ctx, client, rc, bc, prNum, commentIDNum,
				"cleanup command", environment,
			)
			if err != nil {
				return err
			}
			if fb == nil {
				return nil
			}
			// If cleanup failed, post error feedback before returning
			if fb.Type == feedback.Error {
				if err := postCombinedFeedback(ctx, client, rc, prNum, commentIDNum, fb); err != nil {
					return err
				}
			}
			// Cleanup complete (success case deletes comment, so no feedback needed)
			return nil
		default:
			// Unknown command type, ignore
			continue
		}

		if err != nil {
			return err
		}
		if fb == nil {
			continue
		}

		// For new comments, filter out "already approved" warnings
		// Just acknowledge with eyes reaction instead
		if isNewComment && fb.Type == feedback.Warning &&
			fb.Message != "" && strings.Contains(fb.Message, "Already Approved") {
			// Add eyes reaction to acknowledge (user already approved)
			if err := addEyesReaction(ctx, client, rc, commentIDNum); err != nil {
				return err
			}
			continue
		}

		feedbacks = append(feedbacks, fb)
	}

	// If no actionable feedback (e.g., only "already approved" for new comment), return early
	if len(feedbacks) == 0 {
		return nil
	}

	// Add eyes reaction to acknowledge command execution
	if err := addEyesReaction(ctx, client, rc, commentIDNum); err != nil {
		return err
	}

	// Combine all feedback and post once
	combinedFeedback := feedback.CombineFeedback(feedbacks, bc.QuietSuccess)

	return postCombinedFeedback(ctx, client, rc, prNum, commentIDNum, combinedFeedback)
}

// loadRuntimeConfig loads runtime configuration from flags and environment
func loadRuntimeConfig(cmd *cobra.Command) *RuntimeConfig {
	rc := &RuntimeConfig{}

	// Get values from flags
	rc.Token, _ = cmd.Flags().GetString(flagToken)
	rc.CommentBody, _ = cmd.Flags().GetString(flagCommentBody)
	rc.CommentID, _ = cmd.Flags().GetString(flagCommentID)
	rc.PRNumber, _ = cmd.Flags().GetString(flagPRNumber)
	rc.RepoOwner, _ = cmd.Flags().GetString(flagRepoOwner)
	rc.RepoName, _ = cmd.Flags().GetString(flagRepoName)
	rc.CommentAuthor, _ = cmd.Flags().GetString(flagCommentAuthor)

	// Load from environment if not provided via flags
	loadEnvIfEmpty(&rc.Token, envGitHubToken)
	loadEnvIfEmpty(&rc.CommentBody, envCommentBody)
	loadEnvIfEmpty(&rc.CommentID, envCommentID)
	loadEnvIfEmpty(&rc.CommentAction, envCommentAction)
	loadEnvIfEmpty(&rc.PRNumber, envPRNumber)
	loadEnvIfEmpty(&rc.RepoOwner, envRepoOwner)
	loadEnvIfEmpty(&rc.RepoName, envRepoName)
	loadEnvIfEmpty(&rc.CommentAuthor, envCommentAuthor)
	loadEnvIfEmpty(&rc.GitHubAppPrivateKey, envGitHubAppPrivateKey)
	loadEnvIfEmpty(&rc.GitHubAppClientID, envGitHubAppClientID)
	loadEnvIfEmpty(&rc.GitHubAppID, envGitHubAppID)
	loadEnvIfEmpty(&rc.InstallationID, envInstallationID)
	loadEnvIfEmpty(&rc.APIBaseURL, envAPIBaseURL)

	// Load bot username with default for GitHub App
	loadEnvIfEmpty(&rc.BotUsername, envBotUsername)
	if rc.BotUsername == "" {
		rc.BotUsername = defaultBotUsername
	}

	return rc
}

// loadBotConfig resolves the settings this process starts with.
//
// Every entry point calls this with its own flag set, so the ladder is the
// same whether a comment is answered by the Action, the sweep or the service.
// A command that registered no settings flags simply contributes nothing at
// that layer.
func loadBotConfig(cmd *cobra.Command) (*config.Config, error) {
	bc, err := config.LoadProcess(cmd.Flags())
	if err != nil {
		return nil, NewConfigError(ErrConfigLoad, err)
	}

	// The one thing Smyklot cannot migrate for anyone. A repository's
	// configuration file gets a pull request; this variable may be an Actions
	// variable, which the App has no permission to write, so saying so is all
	// there is to do
	if config.DocumentIsLegacyJSON() {
		logging.From(cmd.Context()).Warn(
			"SMYKLOT_CONFIG is written as JSON; rewrite it as TOML",
			"variable", config.EnvConfig,
		)
	}

	return bc, nil
}

// loadEnvIfEmpty loads environment variable into target if target is empty
func loadEnvIfEmpty(target *string, envVar string) {
	if *target == "" {
		*target = os.Getenv(envVar)
	}
}

// validateConfig validates that all required configuration is present
func validateConfig(rc *RuntimeConfig) error {
	if rc.Token == "" {
		return NewEnvVarError(ErrMissingEnvVar, envGitHubToken)
	}

	return validateCommentInput(rc)
}

// validateCommentInput validates everything about a comment that does not
// depend on how the process authenticated.
//
// The service knows all of this from the delivery payload before it mints a
// token, so it can reject a bad delivery without doing any work first.
func validateCommentInput(rc *RuntimeConfig) error {
	requiredFields := []struct {
		value  string
		envVar string
	}{
		{rc.CommentBody, envCommentBody},
		{rc.CommentID, envCommentID},
		{rc.PRNumber, envPRNumber},
		{rc.RepoOwner, envRepoOwner},
		{rc.RepoName, envRepoName},
		{rc.CommentAuthor, envCommentAuthor},
	}

	for _, field := range requiredFields {
		if field.value == "" {
			return NewEnvVarError(ErrMissingEnvVar, field.envVar)
		}
	}

	// Validate comment body length to prevent DoS
	if len(rc.CommentBody) > maxCommentBodyLength {
		return NewInputError(
			ErrInvalidInput,
			rc.CommentBody,
			errCommentTooLong,
		)
	}

	// Validate repository owner and name format
	if !githubNamePattern.MatchString(rc.RepoOwner) {
		return NewInputError(
			ErrInvalidInput,
			rc.RepoOwner,
			errInvalidRepoName,
		)
	}

	if !githubNamePattern.MatchString(rc.RepoName) {
		return NewInputError(
			ErrInvalidInput,
			rc.RepoName,
			errInvalidRepoName,
		)
	}

	return nil
}

// postFeedback posts a comment and adds a reaction to a PR.
func postFeedback(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	prNum, commentID int,
	message string,
	reaction github.ReactionType,
) error {
	// Only post-comment if the message is not empty
	if message != "" {
		if err := client.PostComment(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			message,
		); err != nil {
			return NewGitHubError(ErrPostComment, err)
		}
	}

	// Remove eyes reaction after the operation completes
	_ = client.RemoveReactionByUser(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		github.ReactionEyes,
		rc.BotUsername,
	)

	// Add final status reaction
	if err := client.AddReaction(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		reaction,
	); err != nil {
		return NewGitHubError(ErrAddReaction, err)
	}

	return nil
}

// addEyesReaction adds an eyes reaction to a comment to acknowledge the command.
func addEyesReaction(ctx context.Context, client *github.Client, rc *RuntimeConfig, commentID int) error {
	if err := client.AddReaction(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		github.ReactionEyes,
	); err != nil {
		return NewGitHubError(ErrAddReaction, err)
	}

	return nil
}

// postOperationFailure posts failure feedback for a failed operation.
func postOperationFailure(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	prNum, commentID int,
	operationErr error,
	feedbackFunc func(string) *feedback.Feedback,
	sentinelErr error,
) error {
	fb := feedbackFunc(operationErr.Error())

	if err := postFeedback(
		ctx,
		client,
		rc,
		prNum,
		commentID,
		fb.Message,
		github.ReactionError,
	); err != nil {
		return err
	}

	return NewGitHubError(sentinelErr, operationErr)
}

// checkUserPermission checks if a user has permission to approve/merge
//
// It first checks CODEOWNERS permissions. If no CODEOWNERS exists (empty),
// it falls back to checking if the user has admin/write repository permissions.
func checkUserPermission(
	ctx context.Context,
	client *github.Client,
	checker *permissions.Checker,
	username, owner, repo, rootPath string,
) (bool, error) {
	// First check CODEOWNERS permissions
	canApprove, err := checker.CanApprove(username, rootPath)
	if err != nil {
		return false, err
	}

	// If user is in CODEOWNERS, grant permission
	if canApprove {
		return true, nil
	}

	// If CODEOWNERS has no approvers (empty file), check admin permissions
	if len(checker.GetApprovers()) == 0 {
		logging.From(ctx).Warn("no CODEOWNERS, falling back to admin permissions", "user", username)
		hasWrite, err := client.HasWritePermission(ctx, owner, repo, username)
		if err != nil {
			return false, err
		}
		return hasWrite, nil
	}

	// CODEOWNERS exists but user is not in it
	return false, nil
}

// handleUnauthorized posts feedback for unauthorized users.
func handleUnauthorized(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	checker *permissions.Checker,
	prNum, commentID int,
) error {
	fb := feedback.NewUnauthorized(rc.CommentAuthor, checker.GetApprovers())

	return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError)
}

// isBotAlreadyApproved checks if the bot has already approved the PR.
// Returns true if bot already approved, false otherwise.
//
// The botUsername parameter should be provided from RuntimeConfig.BotUsername
// to avoid calling GetAuthenticatedUser which fails with GitHub App tokens.
func isBotAlreadyApproved(info *github.PRInfo, botUsername string) bool {
	for _, approver := range info.ApprovedBy {
		if approver == botUsername {
			return true
		}
	}

	return false
}

// handleApprove handles the /approve command.
// executeApprove executes the approve command and returns feedback
//
//nolint:unparam // error return kept for consistent function signature
func executeApprove(ctx context.Context, client *github.Client, rc *RuntimeConfig, bc *config.Config, prNum int) (*feedback.Feedback, error) {
	// Get PR info to check existing approvals and prevent self-approval
	info, err := client.GetPRInfo(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return feedback.NewApprovalFailed(err.Error()), nil
	}

	// Prevent self-approval unless explicitly allowed
	if !bc.AllowSelfApproval && info.Author == rc.CommentAuthor {
		return feedback.NewUnauthorized(
			rc.CommentAuthor,
			[]string{selfApprovalNotAllowed},
		), nil
	}

	// Check if bot already approved the PR (prevents duplicate approvals from edits/reactions)
	if isBotAlreadyApproved(info, rc.BotUsername) {
		// Bot already approved - return feedback (filtered for new comments)
		return feedback.NewAlreadyApproved(rc.BotUsername), nil
	}

	// Check if already approved by the comment author (informational feedback)
	for _, approver := range info.ApprovedBy {
		if approver == rc.CommentAuthor {
			// Already approved - return feedback indicating no action needed
			// This will be filtered out in the main loop for new comments
			return feedback.NewAlreadyApproved(rc.CommentAuthor), nil
		}
	}

	// Approve the PR
	if err := client.ApprovePR(ctx, rc.RepoOwner, rc.RepoName, prNum); err != nil {
		return feedback.NewApprovalFailed(err.Error()), nil
	}

	return feedback.NewApprovalSuccess(rc.CommentAuthor, bc.QuietSuccess), nil
}

// executeMerge executes the merge command with specified method and returns feedback
//
//nolint:unparam // error return kept for consistent function signature
func executeMerge(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	method github.MergeMethod,
	waitForCI bool,
	requiredChecksOnly bool,
	environment commandEnvironment,
) (*feedback.Feedback, error) {
	// Get PR info to check if it's mergeable and get base branch
	info, err := client.GetPRInfo(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}

	// Handle "after CI" modifier - defer merge until CI passes
	if waitForCI {
		return executePendingCIMerge(
			ctx, client, rc, bc, prNum, commentID, method, info,
			requiredChecksOnly, environment,
		)
	}

	// Check if PR is mergeable
	// If blocked by branch protection or unstable (failing checks), try enabling auto-merge
	// Only return "not mergeable" for actual conflicts (dirty state)
	if !info.Mergeable {
		switch info.MergeableState {
		case github.MergeableStateBlocked, github.MergeableStateUnstable:
			// Branch protection or failing checks - enable auto-merge
			return enableAutoMerge(ctx, client, rc, bc, prNum, method)

		case github.MergeableStateDirty:
			// Actual conflicts - cannot merge
			return feedback.NewNotMergeable(), nil

		case github.MergeableStateUnknown, "":
			// Unknown state - try to merge anyway and let it fail with specific error
			// This handles the case where GitHub hasn't computed mergeability yet

		default:
			return feedback.NewNotMergeable(), nil
		}
	}

	// Check if bot already approved the PR (prevents duplicate approvals from edits/reactions)
	botAlreadyApproved := isBotAlreadyApproved(info, rc.BotUsername)

	// Check if user already approved the PR (avoid redundant bot approval)
	userAlreadyApproved := false
	for _, approver := range info.ApprovedBy {
		if approver == rc.CommentAuthor {
			userAlreadyApproved = true
			break
		}
	}

	// Approve the PR if neither bot nor user has already approved
	if !botAlreadyApproved && !userAlreadyApproved {
		if err := client.ApprovePR(ctx, rc.RepoOwner, rc.RepoName, prNum); err != nil {
			return feedback.NewApprovalFailed(err.Error()), nil
		}
	}

	// Check if merge queue is enabled - if so, always use auto-merge
	if info.BaseBranch != "" {
		mergeQueueEnabled, _ := client.IsMergeQueueEnabled(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			info.BaseBranch,
		)
		if mergeQueueEnabled {
			return enableAutoMerge(ctx, client, rc, bc, prNum, method)
		}
	}

	// Merge the PR
	if err := client.MergePR(ctx, rc.RepoOwner, rc.RepoName, prNum, method); err != nil {
		// If merge commits not allowed and using default merge method, try squash first
		if method == github.MergeMethodMerge && strings.Contains(err.Error(), "Merge commits are not allowed") {
			if err := client.MergePR(ctx, rc.RepoOwner, rc.RepoName, prNum, github.MergeMethodSquash); err != nil {
				// Try rebase if squash also fails
				if err := client.MergePR(ctx, rc.RepoOwner, rc.RepoName, prNum, github.MergeMethodRebase); err != nil {
					// Check if we should enable auto-merge instead
					if shouldEnableAutoMerge(err) {
						return enableAutoMerge(ctx, client, rc, bc, prNum, github.MergeMethodRebase)
					}
					return feedback.NewMergeFailed(err.Error()), nil
				}
			}
			// Squash succeeded
			return feedback.NewMergeSuccess(rc.CommentAuthor, bc.QuietSuccess), nil
		}

		// Check if we should enable auto-merge instead of failing
		if shouldEnableAutoMerge(err) {
			return enableAutoMerge(ctx, client, rc, bc, prNum, method)
		}

		return feedback.NewMergeFailed(err.Error()), nil
	}

	return feedback.NewMergeSuccess(rc.CommentAuthor, bc.QuietSuccess), nil
}

// shouldEnableAutoMerge checks if error indicates auto-merge should be enabled
func shouldEnableAutoMerge(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "merge queue") ||
		strings.Contains(errStr, "required status check") ||
		strings.Contains(errStr, "status checks") ||
		strings.Contains(errStr, "required review") ||
		strings.Contains(errStr, "branch protection")
}

// enableAutoMerge enables auto-merge for the PR
func enableAutoMerge(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum int,
	method github.MergeMethod,
) (*feedback.Feedback, error) {
	if err := client.EnableAutoMerge(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
		method,
	); err != nil {
		return feedback.NewAutoMergeFailed(err.Error()), nil
	}

	return feedback.NewAutoMergeEnabled(rc.CommentAuthor, bc.QuietSuccess), nil
}

// executePendingCIMerge handles the "merge after CI" flow
//
// When CI is already passing, merges immediately. Otherwise:
// 1. Approves the PR (if not already approved)
// 2. Adds hourglass reaction to indicate waiting state
// 3. Adds pending-ci label to track state for poll workflow
// 4. Returns pending feedback
//
//nolint:unparam // error return kept for consistent function signature
func executePendingCIMerge(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	method github.MergeMethod,
	info *github.PRInfo,
	requiredChecksOnly bool,
	environment commandEnvironment,
) (*feedback.Feedback, error) {
	// Get PR head SHA for CI status check
	headRef, err := client.GetPRHeadRef(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return feedback.NewMergeFailed("failed to get PR head ref: " + err.Error()), nil
	}
	if environment.pendingCI == nil {
		serviceOwned, ownershipErr := pendingCIServiceOwned(
			ctx, client, rc.RepoOwner, rc.RepoName, prNum, rc.BotUsername,
		)
		if ownershipErr != nil {
			return feedback.NewMergeFailed(ownershipErr.Error()), nil
		}
		if serviceOwned {
			return feedback.NewMergeFailed(
				"the service is still handing off this pending CI request; retry after its waiting reaction is removed",
			), nil
		}
	}
	// The Action has no durable reconciler, so it delegates the wait to GitHub
	// labels. The service records the request and enforces its own policy.
	requiredChecks, err := pendingCIRequiredChecks(
		ctx, client, rc.RepoOwner, rc.RepoName, info.BaseBranch, requiredChecksOnly,
	)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}

	if environment.pendingCI == nil {
		checkStatus, err := client.GetCheckStatus(
			ctx, rc.RepoOwner, rc.RepoName, headRef, requiredChecks,
		)
		if err != nil {
			return feedback.NewMergeFailed("failed to get CI status: " + err.Error()), nil
		}
		if checkStatus.AllPassing {
			serviceOwned, ownershipErr := pendingCIServiceOwned(
				ctx, client, rc.RepoOwner, rc.RepoName, prNum, rc.BotUsername,
			)
			if ownershipErr != nil {
				return feedback.NewMergeFailed(ownershipErr.Error()), nil
			}
			if serviceOwned {
				return nil, nil
			}
			return executeImmediateMerge(ctx, client, rc, bc, prNum, method, info, headRef)
		}
	}

	if failure := pendingCIApprovalAllowed(rc, bc, info); failure != nil {
		return failure, nil
	}

	label := getPendingCILabel(method, requiredChecksOnly)
	if environment.pendingCI == nil {
		if failure := approvePendingCI(
			ctx, client, rc, prNum, pendingCIApprovalRequired(rc, info),
		); failure != nil {
			return failure, nil
		}
		serviceOwned, ownershipErr := pendingCIServiceOwned(
			ctx, client, rc.RepoOwner, rc.RepoName, prNum, rc.BotUsername,
		)
		if ownershipErr != nil {
			return feedback.NewMergeFailed(ownershipErr.Error()), nil
		}
		if serviceOwned {
			return nil, nil
		}
		_ = client.AddReaction(
			ctx, rc.RepoOwner, rc.RepoName, commentID, github.ReactionPendingCI,
		)
		if err := client.AddLabel(ctx, rc.RepoOwner, rc.RepoName, prNum, label); err != nil {
			return feedback.NewMergeFailed("failed to record the pending CI request: " + err.Error()), nil
		}
	} else {
		failures, coordinationErr := activatePendingCI(
			ctx, client, environment.pendingCI, environment.pendingCIActivation,
			pendingCIActivationRequest{
				runtime: rc, owner: rc.RepoOwner, repository: rc.RepoName,
				pullRequest: prNum, commentID: commentID, headSHA: headRef,
				baseBranch: info.BaseBranch, method: method,
				requiredChecksOnly: requiredChecksOnly, label: label,
			},
		)
		if coordinationErr != nil {
			return nil, coordinationErr
		}
		if failures.approval != nil {
			return feedback.NewApprovalFailed(failures.approval.Error()), nil
		}
		if failures.label != nil {
			return feedback.NewMergeFailed(
				"failed to record the pending CI request: " + failures.label.Error(),
			), nil
		}
		if failures.reaction != nil {
			return feedback.NewMergeFailed(
				"failed to record the pending CI request: " + failures.reaction.Error(),
			), nil
		}
		if failures.command != nil {
			return nil, failures.command
		}
		if failures.stale {
			return nil, nil
		}
		if failures.stoodDown {
			return nil, nil
		}
		if failures.ambiguous {
			return feedback.NewMergeFailed(
				"GitHub reported multiple after-CI commands with the same timestamp; reissue the command to choose the intended merge method",
			), nil
		}
	}

	// Return pending feedback
	methodName := getMergeMethodName(method)

	return feedback.NewPendingCI(rc.CommentAuthor, methodName, bc.QuietPending), nil
}

// executeImmediateMerge performs the actual merge when CI has already passed
//
//nolint:unparam // error return kept for consistent function signature
func executeImmediateMerge(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum int,
	method github.MergeMethod,
	info *github.PRInfo,
	expectedHead string,
) (*feedback.Feedback, error) {
	// Prevent self-approval unless explicitly allowed
	if !bc.AllowSelfApproval && info.Author == rc.CommentAuthor {
		return feedback.NewUnauthorized(
			rc.CommentAuthor,
			[]string{selfApprovalNotAllowed},
		), nil
	}

	// Check if bot already approved the PR
	botAlreadyApproved := isBotAlreadyApproved(info, rc.BotUsername)

	// Check if user already approved the PR
	userAlreadyApproved := false
	for _, approver := range info.ApprovedBy {
		if approver == rc.CommentAuthor {
			userAlreadyApproved = true

			break
		}
	}

	// Approve the PR if neither bot nor user has already approved
	if !botAlreadyApproved && !userAlreadyApproved {
		if err := client.ApprovePR(ctx, rc.RepoOwner, rc.RepoName, prNum); err != nil {
			return feedback.NewApprovalFailed(err.Error()), nil
		}
	}

	// Merge the PR
	if err := client.MergePRAtHead(ctx, rc.RepoOwner, rc.RepoName, prNum, method, expectedHead); err != nil {
		// Try fallback methods if merge commits not allowed
		if method == github.MergeMethodMerge && strings.Contains(err.Error(), "Merge commits are not allowed") {
			if err := client.MergePRAtHead(
				ctx, rc.RepoOwner, rc.RepoName, prNum, github.MergeMethodSquash, expectedHead,
			); err != nil {
				if err := client.MergePRAtHead(
					ctx, rc.RepoOwner, rc.RepoName, prNum, github.MergeMethodRebase, expectedHead,
				); err != nil {
					return feedback.NewMergeFailed(err.Error()), nil
				}
			}

			return feedback.NewMergeSuccess(rc.CommentAuthor, bc.QuietSuccess), nil
		}

		return feedback.NewMergeFailed(err.Error()), nil
	}

	return feedback.NewMergeSuccess(rc.CommentAuthor, bc.QuietSuccess), nil
}

// getPendingCILabel returns the appropriate pending-ci label for the merge method and required flag
func getPendingCILabel(method github.MergeMethod, requiredOnly bool) string {
	if requiredOnly {
		switch method {
		case github.MergeMethodSquash:
			return github.LabelPendingCISquashRequired
		case github.MergeMethodRebase:
			return github.LabelPendingCIRebaseRequired
		default:
			return github.LabelPendingCIMergeRequired
		}
	}

	switch method {
	case github.MergeMethodSquash:
		return github.LabelPendingCISquash
	case github.MergeMethodRebase:
		return github.LabelPendingCIRebase
	default:
		return github.LabelPendingCIMerge
	}
}

// getMergeMethodName returns a human-readable name for the merge method
func getMergeMethodName(method github.MergeMethod) string {
	switch method {
	case github.MergeMethodSquash:
		return "squash"
	case github.MergeMethodRebase:
		return "rebase"
	default:
		return "merge"
	}
}

// executeUnapprove executes the unapprove command and returns feedback
//
//nolint:unparam // error return kept for consistent function signature
func executeUnapprove(ctx context.Context, client *github.Client, rc *RuntimeConfig, bc *config.Config, prNum int) (*feedback.Feedback, error) {
	// Dismiss the review using configured bot username
	if err := client.DismissReviewByUsername(ctx, rc.RepoOwner, rc.RepoName, prNum, rc.BotUsername); err != nil {
		return feedback.NewUnapproveFailed(err.Error()), nil
	}

	return feedback.NewUnapproveSuccess(rc.CommentAuthor, bc.QuietSuccess), nil
}

// executeCleanup executes the cleanup command and returns feedback
//
// Cleanup removes all bot reactions, approvals, and comments from the PR,
// then deletes the triggering comment.
//
//nolint:unparam // error return kept for consistent function signature
func executeCleanup(ctx context.Context, client *github.Client, rc *RuntimeConfig, bc *config.Config, prNum, commentID int) (*feedback.Feedback, error) {
	// Use configured bot username to identify bot's comments
	botUsername := rc.BotUsername

	// Dismiss bot's review if present
	_ = client.DismissReviewByUsername(ctx, rc.RepoOwner, rc.RepoName, prNum, botUsername)

	// Get all comments on the PR
	comments, err := client.GetPRComments(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return feedback.NewCleanupFailed(err.Error()), nil
	}

	// Delete all bot's comments (except the triggering one for now)
	for _, comment := range comments {
		user, ok := comment["user"].(map[string]interface{})
		if !ok {
			continue
		}

		username, ok := user["login"].(string)
		if !ok || username != botUsername {
			continue
		}

		id, ok := comment["id"].(float64)
		if !ok {
			continue
		}

		commentIDInt := int(id)

		// Skip the triggering comment for now (delete it last)
		if commentIDInt == commentID {
			continue
		}

		// Delete bot's comment
		_ = client.DeleteComment(ctx, rc.RepoOwner, rc.RepoName, commentIDInt)
	}

	// Get all reactions on the triggering comment and remove them
	reactions, err := client.GetCommentReactions(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
	)
	if err == nil {
		// Remove all bot's reactions
		for _, reaction := range reactions {
			if reaction.User == botUsername {
				_ = client.RemoveReactionByUser(
					ctx,
					rc.RepoOwner,
					rc.RepoName,
					commentID,
					reaction.Type,
					botUsername,
				)
			}
		}
	}

	// Delete the triggering comment last
	if err := client.DeleteComment(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
	); err != nil {
		return feedback.NewCleanupFailed(err.Error()), nil
	}

	return feedback.NewCleanupSuccess(rc.CommentAuthor, bc.QuietSuccess), nil
}

// postCombinedFeedback posts combined feedback with appropriate reaction
func postCombinedFeedback(ctx context.Context, client *github.Client, rc *RuntimeConfig, prNum, commentID int, fb *feedback.Feedback) error {
	// Map feedback type to reaction
	var reaction github.ReactionType
	switch fb.Type {
	case feedback.Success:
		reaction = github.ReactionSuccess
	case feedback.Error:
		reaction = github.ReactionError
	case feedback.Warning:
		reaction = github.ReactionWarning
	case feedback.Pending:
		reaction = github.ReactionPendingCI
	default:
		reaction = github.ReactionSuccess
	}

	// Post comment if there's a message
	if fb.RequiresComment() {
		if err := client.PostComment(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			fb.Message,
		); err != nil {
			return NewGitHubError(ErrPostComment, err)
		}
	}

	// Remove eyes reaction before adding final status reaction
	_ = client.RemoveReactionByUser(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		github.ReactionEyes,
		rc.BotUsername,
	)

	// Add reaction
	if err := client.AddReaction(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		commentID,
		reaction,
	); err != nil {
		return NewGitHubError(ErrAddReaction, err)
	}

	return nil
}

// deletedCommandsToReport returns the commands from a deleted comment that are
// worth a notification, or nil when the deletion should pass unremarked.
//
// Only commands that change the PR's approval or merge state qualify. Regular
// discussion comments get deleted routinely and are none of the bot's business,
// and /cleanup deletes its own triggering comment.
//
// A comment rejected by validation still reports - /approve /unapprove asked
// for an approval and its deletion is worth the same record as a clean one.
func deletedCommandsToReport(bc *config.Config, parsedCmd commands.Command) []commands.CommandType {
	if bc.DisableDeletedComments {
		return nil
	}

	return parsedCmd.StateChangingCommands()
}

// handleDeletedComment posts a notification that a command comment was deleted.
func handleDeletedComment(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	deletedCommands []commands.CommandType,
) error {
	// Convert PR number and comment ID
	prNum, err := strconv.Atoi(rc.PRNumber)
	if err != nil {
		return NewInputError(ErrInvalidInput, rc.PRNumber, errInvalidPRNum)
	}

	commentIDNum, err := strconv.Atoi(rc.CommentID)
	if err != nil {
		return NewInputError(ErrInvalidInput, rc.CommentID, errInvalidComment)
	}

	// Post feedback about deleted comment
	fb := feedback.NewCommentDeleted(rc.CommentAuthor, commentIDNum, commandNames(deletedCommands))

	return client.PostComment(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
		fb.Message,
	)
}

// commandNames converts command types to their string names for feedback
func commandNames(cmdTypes []commands.CommandType) []string {
	names := make([]string, 0, len(cmdTypes))
	for _, cmdType := range cmdTypes {
		names = append(names, string(cmdType))
	}

	return names
}

// handleHelp handles the /help command.
func handleHelp(ctx context.Context, client *github.Client, rc *RuntimeConfig, prNum, commentID int) error {
	// Add eyes reaction to acknowledge
	if err := addEyesReaction(ctx, client, rc, commentID); err != nil {
		return err
	}

	// Post help feedback
	fb := feedback.NewHelp()

	return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionSuccess)
}

// handleReactions processes reaction-based approvals and merges.
func handleReactions(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	checker *permissions.Checker,
	prNum, commentID int,
	environment commandEnvironment,
) error {
	// Fetch reactions - use PR reactions if commentID equals prNum (PR description),
	// otherwise get comment reactions
	var reactions []github.Reaction
	var err error

	if commentID == prNum {
		// Get reactions on the PR description
		reactions, err = client.GetPRReactions(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
		)
	} else {
		// Get reactions on a comment
		reactions, err = client.GetCommentReactions(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			commentID,
		)
	}

	if err != nil {
		// Don't fail if we can't fetch reactions, just skip
		return nil
	}

	// Fetch current PR labels
	labels, err := client.GetLabels(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
	)
	if err != nil {
		// Don't fail if we can't fetch labels, just skip
		return nil
	}

	// Build maps for quick lookup
	reactionMap := make(map[github.ReactionType]bool)
	labelMap := make(map[string]bool)

	for _, reaction := range reactions {
		// Check if user has permission
		canApprove, err := checkUserPermission(
			ctx,
			client,
			checker,
			reaction.User,
			rc.RepoOwner,
			rc.RepoName,
			rootPath,
		)
		if err != nil || !canApprove {
			continue
		}

		reactionMap[reaction.Type] = true
	}

	for _, label := range labels {
		labelMap[label] = true
	}

	// Handle removed reactions (reconciliation)
	if err := handleRemovedReactions(
		ctx,
		client,
		rc,
		bc,
		prNum,
		reactionMap,
		labelMap,
	); err != nil {
		return err
	}

	// Process each reaction
	for _, reaction := range reactions {
		// Check if user has permission
		canApprove, err := checkUserPermission(
			ctx,
			client,
			checker,
			reaction.User,
			rc.RepoOwner,
			rc.RepoName,
			rootPath,
		)
		if err != nil || !canApprove {
			continue
		}

		// Handle approve reaction
		if reaction.Type == github.ReactionApprove {
			if err := handleReactionApprove(ctx, client, rc, bc, prNum, commentID, reaction.User); err != nil {
				return err
			}
		}

		// Handle merge reaction
		if reaction.Type == github.ReactionMerge {
			if err := handleReactionMerge(ctx, client, rc, bc, prNum, commentID, reaction.User); err != nil {
				return err
			}
		}

		// Handle cleanup reaction
		if reaction.Type == github.ReactionCleanup {
			if err := handleReactionCleanup(
				ctx, client, rc, bc, prNum, commentID, environment,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// handleRemovedReactions handles reactions that were removed.
func handleRemovedReactions(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum int,
	reactionMap map[github.ReactionType]bool,
	labelMap map[string]bool,
) error {
	// Check if approve reaction was removed
	if labelMap[github.LabelReactionApprove] && !reactionMap[github.ReactionApprove] {
		// Approve reaction was removed, unapprove the PR
		if err := client.DismissReviewByUsername(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			rc.BotUsername,
		); err != nil {
			// Don't fail, just log
			logging.From(ctx).Warn("failed to dismiss review after reaction removal", "error", err)
		}

		// Remove the label
		_ = client.RemoveLabel(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			github.LabelReactionApprove,
		)
	}

	// Check if merge reaction was removed
	if labelMap[github.LabelReactionMerge] && !reactionMap[github.ReactionMerge] {
		// Get PR info to check if it's already merged
		info, err := client.GetPRInfo(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
		)
		if err != nil {
			// Don't fail, just log
			logging.From(ctx).Warn("failed to get PR info after reaction removal", "error", err)

			return nil
		}

		// If PR is already merged, post warning (unless disabled)
		if info.State == github.PullRequestClosed {
			if !bc.QuietReactions {
				fb := feedback.NewReactionMergeRemoved()
				_ = client.PostComment(
					ctx,
					rc.RepoOwner,
					rc.RepoName,
					prNum,
					fb.Message,
				)
			}
		}

		// Remove the label
		_ = client.RemoveLabel(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			github.LabelReactionMerge,
		)
	}

	// Check if cleanup reaction was removed
	if labelMap[github.LabelReactionCleanup] && !reactionMap[github.ReactionCleanup] {
		// Cleanup reaction was removed, just remove the label
		// (no action needed since cleanup is one-time operation)
		_ = client.RemoveLabel(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			github.LabelReactionCleanup,
		)
	}

	return nil
}

// handleReactionApprove handles approval via 👍 reaction.
func handleReactionApprove(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	approver string,
) error {
	// Get PR info to check existing approvals and prevent self-approval
	info, err := client.GetPRInfo(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return postOperationFailure(
			ctx,
			client,
			rc,
			prNum,
			commentID,
			err,
			feedback.NewApprovalFailed,
			ErrApprovePR,
		)
	}

	// Prevent self-approval unless explicitly allowed
	if !bc.AllowSelfApproval && info.Author == approver {
		fb := feedback.NewUnauthorized(approver, []string{selfApprovalNotAllowed})
		return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError)
	}

	// Check if bot already approved the PR (prevents duplicate approvals)
	if isBotAlreadyApproved(info, rc.BotUsername) {
		// Bot already approved - skip approval but still add label
		_ = client.AddLabel(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			prNum,
			github.LabelReactionApprove,
		)
		return nil
	}

	// Approve the PR
	if err := client.ApprovePR(ctx, rc.RepoOwner, rc.RepoName, prNum); err != nil {
		return postOperationFailure(
			ctx,
			client,
			rc,
			prNum,
			commentID,
			err,
			feedback.NewApprovalFailed,
			ErrApprovePR,
		)
	}

	// Add label to track reaction-based approval
	_ = client.AddLabel(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
		github.LabelReactionApprove,
	)

	// Post success feedback
	fb := feedback.NewReactionApprovalSuccess(approver, bc.QuietReactions)

	return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionSuccess)
}

// handleReactionMerge handles merge via 🚀 reaction.
func handleReactionMerge(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	author string,
) error {
	// Get PR info to check if it's mergeable and prevent self-approval
	info, err := client.GetPRInfo(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return postOperationFailure(
			ctx,
			client,
			rc,
			prNum,
			commentID,
			err,
			feedback.NewMergeFailed,
			ErrMergePR,
		)
	}

	// Prevent self-approval unless explicitly allowed (merge also approves)
	if !bc.AllowSelfApproval && info.Author == author {
		fb := feedback.NewUnauthorized(author, []string{selfApprovalNotAllowed})
		return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError)
	}

	// Check if PR is mergeable
	if !info.Mergeable {
		return postNotMergeable(ctx, client, rc, prNum, commentID)
	}

	// Check if bot already approved the PR (prevents duplicate approvals from edits/reactions)
	botAlreadyApproved := isBotAlreadyApproved(info, rc.BotUsername)

	// Check if user already approved the PR (avoid redundant bot approval)
	userAlreadyApproved := false
	for _, approver := range info.ApprovedBy {
		if approver == author {
			userAlreadyApproved = true
			break
		}
	}

	// Approve the PR if neither bot nor user has already approved
	if !botAlreadyApproved && !userAlreadyApproved {
		if err := client.ApprovePR(ctx, rc.RepoOwner, rc.RepoName, prNum); err != nil {
			return postOperationFailure(
				ctx,
				client,
				rc,
				prNum,
				commentID,
				err,
				feedback.NewApprovalFailed,
				ErrApprovePR,
			)
		}
	}

	// Merge the PR (using default merge method)
	if err := client.MergePR(ctx, rc.RepoOwner, rc.RepoName, prNum, github.MergeMethodMerge); err != nil {
		// Check if we should enable auto-merge instead
		if shouldEnableAutoMerge(err) {
			if err := client.EnableAutoMerge(
				ctx,
				rc.RepoOwner,
				rc.RepoName,
				prNum,
				github.MergeMethodMerge,
			); err != nil {
				return postOperationFailure(
					ctx,
					client,
					rc,
					prNum,
					commentID,
					err,
					feedback.NewAutoMergeFailed,
					ErrMergePR,
				)
			}

			// Add label to track reaction-based auto-merge
			_ = client.AddLabel(
				ctx,
				rc.RepoOwner,
				rc.RepoName,
				prNum,
				github.LabelReactionMerge,
			)

			// Post auto-merge enabled feedback
			fb := feedback.NewAutoMergeEnabled(author, bc.QuietReactions)
			return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionSuccess)
		}

		return postOperationFailure(
			ctx,
			client,
			rc,
			prNum,
			commentID,
			err,
			feedback.NewMergeFailed,
			ErrMergePR,
		)
	}

	// Add label to track reaction-based merge
	_ = client.AddLabel(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
		github.LabelReactionMerge,
	)

	// Post success feedback
	fb := feedback.NewReactionMergeSuccess(author, bc.QuietReactions)

	return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionSuccess)
}

// handleReactionCleanup handles cleanup via ❤️ reaction.
func handleReactionCleanup(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	environment commandEnvironment,
) error {
	fb, accepted, err := executeCoordinatedCleanup(
		ctx, client, rc, bc, prNum, commentID,
		"cleanup reaction", environment,
	)
	if err != nil {
		return err
	}
	if !accepted {
		return nil
	}

	// If cleanup failed, post error feedback
	if fb.Type == feedback.Error {
		return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError)
	}

	// Cleanup succeeded - the comment and reactions are already deleted by executeCleanup
	// Remove the label to track that cleanup completed
	_ = client.RemoveLabel(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
		prNum,
		github.LabelReactionCleanup,
	)

	return nil
}

func executeCoordinatedCleanup(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	reason string,
	environment commandEnvironment,
) (*feedback.Feedback, bool, error) {
	var result *feedback.Feedback
	operation := func() error {
		var err error
		result, err = executeCleanup(ctx, client, rc, bc, prNum, commentID)

		return err
	}
	if environment.pendingCI == nil {
		err := operation()

		return result, true, err
	}
	accepted, err := environment.pendingCI.cancelAndRun(
		ctx, prNum, reason, operation,
	)

	return result, accepted, err
}

// postNotMergeable posts feedback when PR is not mergeable.
func postNotMergeable(ctx context.Context, client *github.Client, rc *RuntimeConfig, prNum, commentID int) error {
	fb := feedback.NewNotMergeable()

	return postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionWarning)
}

// sanitizeCommentBody redacts sensitive information from comment body
func sanitizeCommentBody(body string, maxLen int) string {
	// Redact potential secrets (tokens, API keys, passwords)
	sensitivePattern := regexp.MustCompile(`(?i)(token|key|secret|password|bearer)[:=]\s*\S+`)
	sanitized := sensitivePattern.ReplaceAllString(body, "$1: [REDACTED]")

	// Truncate if too long
	if len(sanitized) > maxLen {
		return sanitized[:maxLen] + "..."
	}

	return sanitized
}

// getInstallationToken generates a GitHub App installation token if credentials are provided.
//
// Returns an empty string if GitHub App credentials are not configured.
// Returns the token on success.
func getInstallationToken(rc *RuntimeConfig) (string, error) {
	// Check if GitHub App credentials are provided
	if rc.GitHubAppPrivateKey == "" || rc.InstallationID == "" {
		return "", nil
	}

	// Determine which ID to use (ClientID is preferred, fallback to AppID)
	clientID := rc.GitHubAppClientID
	if clientID == "" {
		clientID = rc.GitHubAppID
	}

	if clientID == "" {
		return "", nil
	}

	// Convert installation ID to int64
	installationID, err := strconv.ParseInt(rc.InstallationID, 10, 64)
	if err != nil {
		return "", NewInputError(ErrInvalidInput, rc.InstallationID, errInvalidInstallID)
	}

	// Minting goes through the same store the service uses. Two implementations
	// had already drifted once: this path never passed the API base URL, so a
	// GitHub Enterprise install minted its token against public GitHub while
	// every other call went to the enterprise host
	tokens, err := githubapp.NewTokenStore(
		clientID, []byte(rc.GitHubAppPrivateKey), rc.APIBaseURL, githubapp.DefaultMintTimeout)
	if err != nil {
		return "", NewGitHubError(ErrGitHubAppAuth, err)
	}

	token, err := tokens.InstallationToken(installationID)
	if err != nil {
		return "", NewGitHubError(ErrGitHubAppAuth, err)
	}

	return token, nil
}

// appendStepSummary adds one note to the GitHub Actions step summary.
//
// This is the only place the summary file is opened. Outside Actions there is
// no such file, and nothing is written.
func appendStepSummary(note string) error {
	summaryFile := os.Getenv(envStepSummary)
	if summaryFile == "" {
		// Not running in GitHub Actions, skip
		return nil
	}

	//nolint:gosec // summaryFile is from the trusted GitHub Actions environment
	file, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return NewGitHubError(ErrStepSummary, err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := file.WriteString(note); err != nil {
		return NewGitHubError(ErrStepSummary, err)
	}

	return nil
}

// writeStepSummary writes the effective configuration to GitHub Actions step summary.
func writeStepSummary(rc *RuntimeConfig, bc *config.Config) error {
	// Rendered before anything is opened, so appendStepSummary stays the one
	// place that knows how to write to the summary
	var rendered strings.Builder

	tmpl, err := template.New(summaryTemplateName).Parse(stepSummaryTemplate)
	if err != nil {
		return NewGitHubError(ErrStepSummary, err)
	}

	var allowedCommands string
	if len(bc.AllowedCommands) > 0 {
		allowedCommands = strings.Join(bc.AllowedCommands, ", ")
	}

	data := stepSummaryData{
		RepoOwner:              rc.RepoOwner,
		RepoName:               rc.RepoName,
		PRNumber:               rc.PRNumber,
		CommentID:              rc.CommentID,
		CommentAuthor:          rc.CommentAuthor,
		CommentBody:            sanitizeCommentBody(rc.CommentBody, 100),
		GitHubApp:              rc.GitHubAppPrivateKey != "",
		AppID:                  rc.GitHubAppID,
		InstallationID:         rc.InstallationID,
		QuietSuccess:           bc.QuietSuccess,
		QuietReactions:         bc.QuietReactions,
		CommandPrefix:          bc.CommandPrefix,
		DisableMentions:        bc.DisableMentions,
		DisableBareCommands:    bc.DisableBareCommands,
		DisableUnapprove:       bc.DisableUnapprove,
		DisableReactions:       bc.DisableReactions,
		DisableDeletedComments: bc.DisableDeletedComments,
		AllowSelfApproval:      bc.AllowSelfApproval,
		AllowedCommands:        allowedCommands,
		CommandAliases:         bc.CommandAliases,
	}

	if err := tmpl.Execute(&rendered, data); err != nil {
		return NewGitHubError(ErrStepSummary, err)
	}

	return appendStepSummary(rendered.String())
}
