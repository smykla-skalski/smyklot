// Package bot is one pull request, one command, one answer.
//
// It is the layer all three entry points stand on - the Action, the cron poll
// and the webhook service - and it is the layer that has to work with none of
// the rest of them present: a repository running the Action has no panel, no
// store to sweep and no worker to schedule. So nothing here reaches upward. A
// caller that needs something from the service it runs inside says so in an
// interface and is handed an implementation.
package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/commands"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/permissions"
)

const (
	EnvGitHubToken         = "GITHUB_TOKEN" //nolint:gosec // Environment variable name, not a credential
	EnvCommentBody         = "COMMENT_BODY"
	EnvCommentID           = "COMMENT_ID"
	EnvCommentRevision     = "COMMENT_UPDATED_AT"
	EnvCommentAction       = "COMMENT_ACTION"
	EnvPRNumber            = "PR_NUMBER"
	EnvRepoOwner           = "REPO_OWNER"
	EnvRepoName            = "REPO_NAME"
	EnvCommentAuthor       = "COMMENT_AUTHOR"
	EnvGitHubAppPrivateKey = "GITHUB_APP_PRIVATE_KEY" //nolint:gosec // Environment variable name, not a credential
	EnvGitHubAppClientID   = "GITHUB_APP_CLIENT_ID"   //nolint:gosec // Environment variable name, not a credential
	EnvGitHubAppID         = "GITHUB_APP_ID"          //nolint:gosec // Environment variable name, not a credential
	EnvInstallationID      = "GITHUB_INSTALLATION_ID"
	EnvBotUsername         = "SMYKLOT_BOT_USERNAME"
	EnvAPIBaseURL          = "SMYKLOT_GITHUB_API_URL"
	rootPath               = "/"
	commentActionCreated   = "created"
	commentActionDeleted   = "deleted"
	summaryTemplateName    = "summary"
	errInvalidPRNum        = "invalid PR number"
	errInvalidComment      = "invalid comment ID"
	errInvalidInstallID    = "invalid installation ID"
	errCommentTooLong      = "comment body exceeds maximum length"
	errInvalidRepoName     = "invalid repository owner or name"
	MaxCommentBodyLength   = 10000 // 10KB - cap on untrusted comment bodies
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
| Allow Draft Merges | ` + "`{{.AllowDraftMerges}}`" + ` |
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

// githubNamePattern validates GitHub repository and owner names
// Allows: alphanumeric, hyphens, underscores, dots (e.g., .dotfiles, foo_bar, foo-bar)
var githubNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ExecuteComment runs whatever a comment asks for, whichever entry point
// delivered it.
//
// The Action and the webhook service differ only in how they arrive here: one
// reads environment variables a workflow set, the other a signed delivery. From
// this point the two are the same code, which is what keeps their results the
// same for the same comment.
func ExecuteComment(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
) error {
	return ExecuteCommentWithEnvironment(ctx, client, rc, bc, CommandEnvironment{})
}

func ExecuteCommentWithEnvironment(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	environment CommandEnvironment,
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
		ReactionError,
		rc.BotUsername,
	)

	// Fetch CODEOWNERS content from GitHub API
	codeownersContent, err := client.GetCodeowners(
		ctx,
		rc.RepoOwner,
		rc.RepoName,
	)
	if err != nil {
		return NewGitHubError(errGetCodeowners, err)
	}

	// Initialize permission checker from content
	checker, err := permissions.NewCheckerFromContent(codeownersContent, client)
	if err != nil {
		return NewGitHubError(errInitPermissions, err)
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
	canApprove, err := CheckUserPermission(
		ctx,
		client,
		checker,
		rc.CommentAuthor,
		rc.RepoOwner,
		rc.RepoName,
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

// ValidateConfig validates that all required configuration is present
func ValidateConfig(rc *RuntimeConfig) error {
	if rc.Token == "" {
		return NewInputError(errMissingEnvVar, EnvGitHubToken, "")
	}

	return ValidateCommentInput(rc)
}

// ValidateCommentInput validates everything about a comment that does not
// depend on how the process authenticated.
//
// The service knows all of this from the delivery payload before it mints a
// token, so it can reject a bad delivery without doing any work first.
func ValidateCommentInput(rc *RuntimeConfig) error {
	requiredFields := []struct {
		value  string
		envVar string
	}{
		{rc.CommentBody, EnvCommentBody},
		{rc.CommentID, EnvCommentID},
		{rc.PRNumber, EnvPRNumber},
		{rc.RepoOwner, EnvRepoOwner},
		{rc.RepoName, EnvRepoName},
		{rc.CommentAuthor, EnvCommentAuthor},
	}

	for _, field := range requiredFields {
		if field.value == "" {
			return NewInputError(errMissingEnvVar, field.envVar, "")
		}
	}

	// Validate comment body length to prevent DoS
	if len(rc.CommentBody) > MaxCommentBodyLength {
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
	environment CommandEnvironment,
) (*feedback.Feedback, error) {
	// Get PR info to check if it's mergeable and get base branch
	info, err := client.GetPRInfo(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}
	if bc.AllowDraftMerges {
		revision, revisionErr := draftMergeCommandRevision(
			ctx, client, rc, commentID, environment,
		)
		if revisionErr != nil {
			return feedback.NewMergeFailed(revisionErr.Error()), nil
		}
		if revisionErr = ValidateDraftMergeAuthorization(
			ctx, client, rc.RepoOwner, rc.RepoName, prNum, revision,
		); revisionErr != nil {
			return feedback.NewMergeFailed(revisionErr.Error()), nil
		}
		environment.DraftMergeRevision = revision
	}

	// Handle "after CI" modifier - defer merge until CI passes
	if waitForCI {
		return executePendingCIMerge(
			ctx, client, rc, bc, prNum, commentID, method, info,
			requiredChecksOnly, environment,
		)
	}
	if environment.PendingCI != nil {
		return executeCoordinatedMerge(
			ctx, client, rc, bc, prNum, commentID, method,
			requiredChecksOnly, environment,
		)
	}

	return executeImmediateCommandMerge(
		ctx, client, rc, bc, prNum, commentID, method, info,
		environment.DraftMergeRevision,
	)
}

func executeCoordinatedMerge(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	method github.MergeMethod,
	requiredChecksOnly bool,
	environment CommandEnvironment,
) (*feedback.Feedback, error) {
	var result *feedback.Feedback
	accepted, err := environment.PendingCI.cancelAndRun(
		ctx,
		prNum,
		"superseded by an immediate merge command",
		func() error {
			var operationErr error
			coordinated := environment
			coordinated.PendingCI = nil
			result, operationErr = executeMerge(
				ctx, client, rc, bc, prNum, commentID, method,
				false, requiredChecksOnly, coordinated,
			)

			return operationErr
		},
	)
	if err != nil {
		if errors.Is(err, pendingci.ErrAmbiguousSourceRevision) {
			return feedback.NewMergeFailed(err.Error()), nil
		}

		return nil, err
	}
	if !accepted {
		return feedback.NewMergeFailed(
			"a newer merge command or draft transition superseded this command; reissue the command to confirm the merge",
		), nil
	}

	return result, nil
}

func executeImmediateCommandMerge(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	method github.MergeMethod,
	info *github.PRInfo,
	sourceRevision string,
) (*feedback.Feedback, error) {
	if failure := PendingCIApprovalAllowed(rc, bc, info); failure != nil {
		return failure, nil
	}
	authorize := commandMergeAuthorizer(
		ctx, client, rc, prNum, commentID, sourceRevision,
	)
	if err := authorize(); err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}
	var err error
	info, err = prepareDraftMerge(
		ctx, client, rc.RepoOwner, rc.RepoName, prNum, bc.AllowDraftMerges, info,
	)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}
	if err := authorize(); err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}
	if failure := approveMergeIfNeeded(ctx, client, rc, prNum, info); failure != nil {
		return failure, nil
	}

	// Check if PR is mergeable
	// If blocked by branch protection or unstable (failing checks), try enabling auto-merge
	// Only return "not mergeable" for actual conflicts (dirty state)
	if !info.Mergeable {
		switch info.MergeableState {
		case github.MergeableStateBlocked, github.MergeableStateUnstable:
			// Branch protection or failing checks - enable auto-merge
			return enableAutoMerge(ctx, client, rc, bc, prNum, method, authorize)

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

	// Check if merge queue is enabled - if so, always use auto-merge
	if info.BaseBranch != "" {
		mergeQueueEnabled, _ := client.IsMergeQueueEnabled(
			ctx,
			rc.RepoOwner,
			rc.RepoName,
			info.BaseBranch,
		)
		if mergeQueueEnabled {
			return enableAutoMerge(ctx, client, rc, bc, prNum, method, authorize)
		}
	}

	return mergeCommandPR(ctx, client, rc, bc, prNum, method, authorize)
}

func approveMergeIfNeeded(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	prNum int,
	info *github.PRInfo,
) *feedback.Feedback {
	botAlreadyApproved := isBotAlreadyApproved(info, rc.BotUsername)
	userAlreadyApproved := slices.Contains(info.ApprovedBy, rc.CommentAuthor)
	if botAlreadyApproved || userAlreadyApproved {
		return nil
	}
	if err := client.ApprovePR(ctx, rc.RepoOwner, rc.RepoName, prNum); err != nil {
		return feedback.NewApprovalFailed(err.Error())
	}

	return nil
}

func mergeCommandPR(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum int,
	method github.MergeMethod,
	authorize mergeAuthorizer,
) (*feedback.Feedback, error) {
	merge := func(mergeMethod github.MergeMethod) error {
		return runMergeAuthorizedEffect(
			authorize,
			func() error {
				return client.MergePR(ctx, rc.RepoOwner, rc.RepoName, prNum, mergeMethod)
			},
		)
	}
	if err := merge(method); err != nil {
		// If merge commits not allowed and using default merge method, try squash first
		if method == github.MergeMethodMerge && strings.Contains(err.Error(), "Merge commits are not allowed") {
			if err := merge(github.MergeMethodSquash); err != nil {
				// Try rebase if squash also fails
				if err := merge(github.MergeMethodRebase); err != nil {
					// Check if we should enable auto-merge instead
					if shouldEnableAutoMerge(err) {
						return enableAutoMerge(
							ctx, client, rc, bc, prNum, github.MergeMethodRebase, authorize,
						)
					}
					return feedback.NewMergeFailed(err.Error()), nil
				}
			}
			// Squash succeeded
			return feedback.NewMergeSuccess(rc.CommentAuthor, bc.QuietSuccess), nil
		}

		// Check if we should enable auto-merge instead of failing
		if shouldEnableAutoMerge(err) {
			return enableAutoMerge(ctx, client, rc, bc, prNum, method, authorize)
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
	authorize mergeAuthorizer,
) (*feedback.Feedback, error) {
	err := runMergeAuthorizedEffect(
		authorize,
		func() error {
			return client.EnableAutoMerge(
				ctx, rc.RepoOwner, rc.RepoName, prNum, method,
			)
		},
	)
	if err != nil {
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
	environment CommandEnvironment,
) (*feedback.Feedback, error) {
	headRef, err := client.GetPRHeadRef(ctx, rc.RepoOwner, rc.RepoName, prNum)
	if err != nil {
		return feedback.NewMergeFailed("failed to get PR head ref: " + err.Error()), nil
	}
	if environment.PendingCI == nil {
		result, finished, actionErr := evaluateActionPendingCI(
			ctx, client, rc, bc, prNum, commentID, method, info, headRef, requiredChecksOnly,
			environment.DraftMergeRevision,
		)
		if actionErr != nil || finished {
			return result, actionErr
		}
	}

	mode := storage.PendingCIModeLabels
	if environment.PendingCI != nil && environment.PendingCIMode != nil {
		resolvedMode, modeErr := environment.PendingCIMode.PendingCIMode(ctx, info.BaseBranch)
		if modeErr != nil {
			return feedback.NewMergeFailed(modeErr.Error()), nil
		}
		mode = resolvedMode
	}

	if failure := PendingCIApprovalAllowed(rc, bc, info); failure != nil {
		return failure, nil
	}
	if environment.PendingCI == nil {
		info, err = prepareDraftMerge(
			ctx, client, rc.RepoOwner, rc.RepoName, prNum, bc.AllowDraftMerges, info,
		)
		if err != nil {
			return feedback.NewMergeFailed(err.Error()), nil
		}
	}

	label := getPendingCILabel(method, requiredChecksOnly)
	if environment.PendingCI == nil {
		result, finished := recordActionPendingCI(
			ctx, client, rc, bc, prNum, commentID, method, requiredChecksOnly,
			info, label, environment.DraftMergeRevision,
		)
		if finished {
			return result, nil
		}
	} else {
		result, finished, activationErr := recordServicePendingCI(
			ctx, client, rc, bc, prNum, commentID, method, info, headRef,
			requiredChecksOnly, label, mode, environment,
		)
		if activationErr != nil || finished {
			return result, activationErr
		}
	}

	methodName := getMergeMethodName(method)

	return feedback.NewPendingCI(rc.CommentAuthor, methodName, bc.QuietPending), nil
}

func evaluateActionPendingCI(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	method github.MergeMethod,
	info *github.PRInfo,
	headRef string,
	requiredChecksOnly bool,
	sourceRevision string,
) (*feedback.Feedback, bool, error) {
	serviceOwned, err := PendingCIServiceOwned(
		ctx, client, rc.RepoOwner, rc.RepoName, prNum, rc.BotUsername,
	)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), true, nil
	}
	if serviceOwned {
		return feedback.NewMergeFailed(
			"the service is still handing off this pending CI request; retry after its waiting reaction is removed",
		), true, nil
	}

	requiredChecks, err := pendingCIRequiredChecks(
		ctx, client, rc.RepoOwner, rc.RepoName, info.BaseBranch, requiredChecksOnly,
	)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), true, nil
	}
	checkStatus, err := client.GetCheckStatus(
		ctx, rc.RepoOwner, rc.RepoName, headRef, requiredChecks,
	)
	if err != nil {
		return feedback.NewMergeFailed("failed to get CI status: " + err.Error()), true, nil
	}
	if !checkStatus.AllPassing {
		return nil, false, nil
	}

	serviceOwned, err = PendingCIServiceOwned(
		ctx, client, rc.RepoOwner, rc.RepoName, prNum, rc.BotUsername,
	)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), true, nil
	}
	if serviceOwned {
		return nil, true, nil
	}
	authorize := commandMergeAuthorizer(
		ctx, client, rc, prNum, commentID, sourceRevision,
	)
	result, mergeErr := executeImmediateMerge(
		ctx, client, rc, bc, prNum, method, info, headRef, authorize,
	)

	return result, true, mergeErr
}

func recordActionPendingCI(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	method github.MergeMethod,
	requiredChecksOnly bool,
	info *github.PRInfo,
	label string,
	sourceRevision string,
) (*feedback.Feedback, bool) {
	resolvedRevision, err := actionPendingCISourceRevision(
		ctx, client, rc, commentID, sourceRevision,
	)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), true
	}
	authorize := actionPendingCIAuthorizer(
		ctx, client, rc, prNum, commentID, resolvedRevision,
	)
	if err := authorize(); err != nil {
		return feedback.NewMergeFailed(err.Error()), true
	}
	if failure := approvePendingCI(
		ctx, client, rc, prNum, PendingCIApprovalRequired(rc, info),
	); failure != nil {
		return failure, true
	}
	serviceOwned, err := PendingCIServiceOwned(
		ctx, client, rc.RepoOwner, rc.RepoName, prNum, rc.BotUsername,
	)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), true
	}
	if serviceOwned {
		return nil, true
	}
	if err := publishActionPendingCI(
		ctx, client, rc, bc, prNum, commentID, method, requiredChecksOnly,
		label, resolvedRevision, authorize,
	); err != nil {
		return feedback.NewMergeFailed(err.Error()), true
	}

	return nil, false
}

func actionPendingCISourceRevision(
	ctx context.Context,
	client *github.Client,
	runtime *RuntimeConfig,
	commentID int,
	sourceRevision string,
) (string, error) {
	if sourceRevision != "" {
		return sourceRevision, nil
	}
	comment, err := client.GetIssueComment(
		ctx, runtime.RepoOwner, runtime.RepoName, int64(commentID),
	)
	if err != nil {
		return "", fmt.Errorf("read pending CI command revision: %w", err)
	}

	return comment.UpdatedAt, nil
}

func actionPendingCIAuthorizer(
	ctx context.Context,
	client *github.Client,
	runtime *RuntimeConfig,
	pullRequest, commentID int,
	sourceRevision string,
) mergeAuthorizer {
	authorizeCommand := commandMergeAuthorizer(
		ctx, client, runtime, pullRequest, commentID, sourceRevision,
	)

	return func() error {
		if err := authorizeCommand(); err != nil {
			return err
		}
		info, err := client.GetPRInfo(
			ctx, runtime.RepoOwner, runtime.RepoName, pullRequest,
		)
		if err != nil {
			return fmt.Errorf("revalidate pending CI pull request: %w", err)
		}
		if info.Draft {
			return fmt.Errorf(
				"%w: pull request returned to draft while the pending CI request was being recorded; reissue the command",
				pendingci.ErrStaleSourceRevision,
			)
		}

		return nil
	}
}

func publishActionPendingCI(
	ctx context.Context,
	client *github.Client,
	runtime *RuntimeConfig,
	botConfig *config.Config,
	pullRequest, commentID int,
	method github.MergeMethod,
	requiredOnly bool,
	label, sourceRevision string,
	authorize mergeAuthorizer,
) (publishErr error) {
	if err := removeActionPendingCILabel(
		ctx, client, runtime.RepoOwner, runtime.RepoName, pullRequest, label,
	); err != nil {
		return fmt.Errorf("disarm the previous pending CI request: %w", err)
	}
	exclusion := actionPendingCIArtifactExclusion{commentID: commentID}
	defer func() {
		if publishErr == nil {
			return
		}
		publishErr = repairActionPendingCILabel(
			ctx, client, botConfig, runtime.RepoOwner, runtime.RepoName,
			pullRequest, method, requiredOnly, runtime.BotUsername, label,
			exclusion, publishErr,
		)
	}()
	for _, reaction := range []github.ReactionType{
		ReactionPendingCIAction, ReactionPendingCI, ReactionPendingCIRejected,
	} {
		if err := client.RemoveReactionByUser(
			ctx, runtime.RepoOwner, runtime.RepoName, commentID,
			reaction, runtime.BotUsername,
		); err != nil {
			return fmt.Errorf("rotate the pending CI command fence: %w", err)
		}
	}
	closed, err := client.AddReactionState(
		ctx, runtime.RepoOwner, runtime.RepoName, commentID, ReactionPendingCIRejected,
	)
	if err != nil {
		return fmt.Errorf("close the pending CI command gate: %w", err)
	}
	if closed.ID == 0 {
		return errors.New("GitHub returned an incomplete pending CI command gate; reissue the command")
	}
	fence, err := client.AddReactionState(
		ctx, runtime.RepoOwner, runtime.RepoName, commentID, ReactionPendingCI,
	)
	if err != nil {
		return fmt.Errorf("record the pending CI command fence: %w", err)
	}
	if err := validateActionPendingCIMarker(fence, sourceRevision); err != nil {
		return err
	}
	exclusion.fenceID = fence.ID
	if err := authorize(); err != nil {
		return fmt.Errorf("revalidate pending CI command: %w", err)
	}
	activation, err := client.AddReactionState(
		ctx, runtime.RepoOwner, runtime.RepoName, commentID, ReactionPendingCIAction,
	)
	if err != nil {
		return fmt.Errorf("activate the pending CI command: %w", err)
	}
	if err := validateActionPendingCIActivation(fence, activation); err != nil {
		return err
	}
	if err := removeActionPendingCIReaction(
		ctx, client, runtime.RepoOwner, runtime.RepoName, commentID, closed.ID,
	); err != nil {
		return fmt.Errorf("open the pending CI command gate: %w", err)
	}
	if err := client.AddLabel(
		ctx, runtime.RepoOwner, runtime.RepoName, pullRequest, label,
	); err != nil {
		return fmt.Errorf("arm the pending CI request: %w", err)
	}

	return nil
}

func validateActionPendingCIActivation(fence, activation github.Reaction) error {
	if activation.ID == 0 || activation.CreatedAt.IsZero() ||
		!newerActionPendingCIMarker(activation, fence) {
		return errors.New(
			"GitHub returned an incomplete pending CI command activation; reissue the command",
		)
	}

	return nil
}

func removeActionPendingCILabel(
	ctx context.Context,
	client actionPendingCILabeler,
	owner, repository string,
	pullRequest int,
	label string,
) error {
	err := client.RemoveLabel(ctx, owner, repository, pullRequest, label)
	var apiErr *github.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil
	}

	return err
}

type actionPendingCILabeler interface {
	RemoveLabel(context.Context, string, string, int, string) error
}

func recordServicePendingCI(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	method github.MergeMethod,
	info *github.PRInfo,
	headRef string,
	requiredChecksOnly bool,
	label string,
	mode storage.PendingCIMode,
	environment CommandEnvironment,
) (*feedback.Feedback, bool, error) {
	artifactKind := pendingci.ArtifactLabel
	if mode == storage.PendingCIModeChecks {
		artifactKind = pendingci.ArtifactCheck
	}
	failures, err := activatePendingCI(
		ctx, client, environment.PendingCI, environment.PendingCIActivation,
		PendingCIActivationRequest{
			Runtime: rc, Owner: rc.RepoOwner, Repository: rc.RepoName,
			PullRequest: prNum, PullRequestTitle: info.Title,
			CommentID: commentID, HeadSHA: headRef,
			BaseBranch: info.BaseBranch, Method: method,
			RequiredChecksOnly: requiredChecksOnly, Label: label,
			ArtifactKind: artifactKind, AllowDraftMerges: bc.AllowDraftMerges,
		},
	)
	if err != nil {
		return nil, true, err
	}

	return pendingCIActivationOutcome(failures)
}

func pendingCIActivationOutcome(
	failures pendingCIActivationErrors,
) (*feedback.Feedback, bool, error) {
	if failures.Ready != nil {
		return feedback.NewMergeFailed(failures.Ready.Error()), true, nil
	}
	if failures.Approval != nil {
		return feedback.NewApprovalFailed(failures.Approval.Error()), true, nil
	}
	if failures.Label != nil {
		return feedback.NewMergeFailed(
			"failed to record the pending CI request: " + failures.Label.Error(),
		), true, nil
	}
	if failures.Check != nil {
		return feedback.NewMergeFailed(
			"failed to record the pending CI check: " + failures.Check.Error(),
		), true, nil
	}
	if failures.Reaction != nil {
		return feedback.NewMergeFailed(
			"failed to record the pending CI request: " + failures.Reaction.Error(),
		), true, nil
	}
	if failures.Command != nil {
		return nil, true, failures.Command
	}
	if failures.Stale || failures.StoodDown {
		return nil, true, nil
	}
	if failures.Ambiguous {
		return feedback.NewMergeFailed(
			"GitHub reported multiple after-CI commands with the same timestamp; reissue the command to choose the intended merge method",
		), true, nil
	}

	return nil, false, nil
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
	authorize mergeAuthorizer,
) (*feedback.Feedback, error) {
	// Prevent self-approval unless explicitly allowed
	if !bc.AllowSelfApproval && info.Author == rc.CommentAuthor {
		return feedback.NewUnauthorized(
			rc.CommentAuthor,
			[]string{selfApprovalNotAllowed},
		), nil
	}
	if err := authorize(); err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}
	info, err := prepareDraftMerge(
		ctx, client, rc.RepoOwner, rc.RepoName, prNum, bc.AllowDraftMerges, info,
	)
	if err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}
	if err := authorize(); err != nil {
		return feedback.NewMergeFailed(err.Error()), nil
	}

	if failure := approveMergeIfNeeded(ctx, client, rc, prNum, info); failure != nil {
		return failure, nil
	}

	mergeAtHead := func(mergeMethod github.MergeMethod) error {
		return runMergeAuthorizedEffect(
			authorize,
			func() error {
				return client.MergePRAtHead(
					ctx, rc.RepoOwner, rc.RepoName, prNum, mergeMethod, expectedHead,
				)
			},
		)
	}

	// Merge the PR
	if err := mergeAtHead(method); err != nil {
		// Try fallback methods if merge commits not allowed
		if method == github.MergeMethodMerge && strings.Contains(err.Error(), "Merge commits are not allowed") {
			if err := mergeAtHead(github.MergeMethodSquash); err != nil {
				if err := mergeAtHead(github.MergeMethodRebase); err != nil {
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
			return LabelPendingCISquashRequired
		case github.MergeMethodRebase:
			return LabelPendingCIRebaseRequired
		default:
			return LabelPendingCIMergeRequired
		}
	}

	switch method {
	case github.MergeMethodSquash:
		return LabelPendingCISquash
	case github.MergeMethodRebase:
		return LabelPendingCIRebase
	default:
		return LabelPendingCIMerge
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
		if comment.User.Login != botUsername {
			continue
		}

		commentIDInt := int(comment.ID)

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

	return PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, ReactionSuccess)
}

func executeCoordinatedCleanup(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	bc *config.Config,
	prNum, commentID int,
	reason string,
	environment CommandEnvironment,
) (*feedback.Feedback, bool, error) {
	var result *feedback.Feedback
	operation := func() error {
		var err error
		result, err = executeCleanup(ctx, client, rc, bc, prNum, commentID)

		return err
	}
	if environment.PendingCI == nil {
		err := operation()

		return result, true, err
	}
	accepted, err := environment.PendingCI.cancelAndRun(
		ctx, prNum, reason, operation,
	)

	return result, accepted, err
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
