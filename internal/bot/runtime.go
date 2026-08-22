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
