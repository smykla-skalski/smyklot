package bot

import (
	"slices"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

const DefaultBotUsername = "smyklot[bot]"

// RuntimeConfig holds the runtime configuration for the action
type RuntimeConfig struct {
	Token               string
	CommentBody         string
	CommentID           string
	CommentRevision     string
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

// isBotAlreadyApproved checks if the bot has already approved the PR.
// Returns true if bot already approved, false otherwise.
//
// The botUsername parameter should be provided from RuntimeConfig.BotUsername
// to avoid calling GetAuthenticatedUser which fails with GitHub App tokens.
func isBotAlreadyApproved(info *github.PRInfo, botUsername string) bool {
	return slices.Contains(info.ApprovedBy, botUsername)
}

// GitHub refuses an approval from a pull request's own author, so a pull
// request the bot opened is one it can never approve.
func isBotAuthored(info *github.PRInfo, botUsername string) bool {
	return botUsername != "" && info.Author == botUsername
}

// botApprovalNeeded reports whether the bot still has to add its own approval.
// A human approval, the commenter's included, does not stand in for it.
func botApprovalNeeded(info *github.PRInfo, botUsername string) bool {
	return !isBotAuthored(info, botUsername) && !isBotAlreadyApproved(info, botUsername)
}
