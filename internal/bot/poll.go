package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/permissions"
)

// RepoFullName is how a repository is named everywhere it is spoken about: in
// a log line, in a cache key, and in the string ParseRepo reads back.
func RepoFullName(owner, name string) string {
	return fmt.Sprintf("%s/%s", owner, name)
}

// ParseRepo parses owner and name from repo string
func ParseRepo(repo string) (string, string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository format (expected owner/name, got %q)", repo)
	}
	return parts[0], parts[1], nil
}

// NewPermissionChecker builds a permission checker for a repository from a
// client that already exists.
//
// The service polls many repositories through one installation client, so it
// needs the checker without a client to go with it.
func NewPermissionChecker(
	ctx context.Context,
	client *github.Client,
	repoOwner, repoName string,
) (*permissions.Checker, error) {
	codeownersContent, err := FetchCodeowners(ctx, client, repoOwner, repoName, nil)
	if err != nil {
		return nil, err
	}

	return CheckerFromCodeowners(codeownersContent, client)
}

// FetchCodeowners reads a repository's CODEOWNERS, or empty when it has none.
//
// It takes what the cache already holds and ignores it: CODEOWNERS is one
// request to read, so there is nothing cheaper to ask first.
func FetchCodeowners(
	ctx context.Context,
	client *github.Client,
	repoOwner, repoName string,
	_ *string,
) (string, error) {
	content, err := client.GetCodeowners(ctx, repoOwner, repoName)
	if err != nil {
		return "", NewGitHubError(errGetCodeowners, err)
	}

	// Log if CODEOWNERS is missing
	if content == "" {
		logging.From(ctx).Warn("no CODEOWNERS, falling back to repository admin permissions",
			"repo", RepoFullName(repoOwner, repoName))
	}

	return content, nil
}

// CheckerFromCodeowners parses CODEOWNERS content into a permission checker.
//
// Kept separate from the fetch so the service can cache the content but still
// bind each checker to the client holding the current installation token.
func CheckerFromCodeowners(content string, client *github.Client) (*permissions.Checker, error) {
	checker, err := permissions.NewCheckerFromContent(content, client)
	if err != nil {
		return nil, NewGitHubError(errInitPermissions, err)
	}

	return checker, nil
}

// PollAllPRs polls and processes reactions on all open PRs
func PollAllPRs(
	ctx context.Context,
	client *github.Client,
	checker *permissions.Checker,
	bc *config.Config,
	repoOwner, repoName string,
	botUsername string,
	environment CommandEnvironment,
	includePendingCI bool,
) error {
	// Named once, here, so every line below carries the repository without
	// each of them having to say so
	ctx = logging.With(ctx, "repo", RepoFullName(repoOwner, repoName))

	logging.From(ctx).Info("polling PR reactions")

	// Get all open PRs
	prs, err := client.GetOpenPRs(ctx, repoOwner, repoName)
	if err != nil {
		return NewGitHubError(ErrGetPRs, err)
	}

	return ProcessAllPRs(
		ctx, client, checker, bc, repoOwner, repoName, botUsername, prs,
		environment, includePendingCI,
	)
}

func ProcessAllPRs(
	ctx context.Context,
	client *github.Client,
	checker *permissions.Checker,
	bc *config.Config,
	repoOwner, repoName, botUsername string,
	prs []map[string]interface{},
	environment CommandEnvironment,
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
	environment CommandEnvironment,
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
		BotUsername:   DefaultBotUsername, // Use default bot username
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
