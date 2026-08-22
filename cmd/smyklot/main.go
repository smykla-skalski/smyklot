package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/spf13/cobra"
)

const (
	appName           = "smyklot" // The binary's own name, in usage output
	flagToken         = "token"
	flagCommentBody   = "comment-body"
	flagCommentID     = "comment-id"
	flagPRNumber      = "pr-number"
	flagRepoOwner     = "repo-owner"
	flagRepoName      = "repo-name"
	flagCommentAuthor = "comment-author"
	descToken         = "GitHub API token" //nolint:gosec // Flag description, not a credential
	descCommentBody   = "PR comment body"
	descCommentID     = "PR comment ID"
	descPRNumber      = "Pull request number"
	descRepoOwner     = "Repository owner"
	descRepoName      = "Repository name"
	descCommentAuthor = "Comment author username"
)

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
	if err := bot.ValidateConfig(rc); err != nil {
		return err
	}

	// Write a step summary with effective configuration
	if err := bot.WriteStepSummary(rc, bc); err != nil {
		// Don't fail if we can't write a summary, just log and continue
		logging.From(ctx).Warn("failed to write step summary", "error", err)
	}

	// Get GitHub App installation token if configured
	token := rc.Token
	installationToken, err := bot.InstallationToken(rc)
	if err != nil {
		return err
	}

	if installationToken != "" {
		token = installationToken
	}

	// Create a GitHub client
	client, err := github.NewClient(token, rc.APIBaseURL)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGitHubClient, err)
	}

	// Layer the repository's own configuration over the workflow's
	//
	// The service reads the same file, so a repository that checks one in gets
	// the same treatment whichever entry point handles the comment
	base := bc

	bc, err = effectiveConfig(ctx, client, rc.RepoOwner, rc.RepoName, base)
	if err != nil {
		if errors.Is(err, bot.ErrRepoConfigInvalid) {
			return reportInvalidRepoConfig(ctx, client, rc, base, err)
		}

		return err
	}

	// The service handles this repository unless the file says otherwise, and
	// two entry points acting on one comment is what that setting exists to
	// stop
	if bot.ActionStandsDown(ctx, bc) {
		return nil
	}

	return bot.ExecuteComment(ctx, client, rc, bc)
}

// loadRuntimeConfig loads runtime configuration from flags and environment
func loadRuntimeConfig(cmd *cobra.Command) *bot.RuntimeConfig {
	rc := &bot.RuntimeConfig{}

	// Get values from flags
	rc.Token, _ = cmd.Flags().GetString(flagToken)
	rc.CommentBody, _ = cmd.Flags().GetString(flagCommentBody)
	rc.CommentID, _ = cmd.Flags().GetString(flagCommentID)
	rc.PRNumber, _ = cmd.Flags().GetString(flagPRNumber)
	rc.RepoOwner, _ = cmd.Flags().GetString(flagRepoOwner)
	rc.RepoName, _ = cmd.Flags().GetString(flagRepoName)
	rc.CommentAuthor, _ = cmd.Flags().GetString(flagCommentAuthor)

	// Load from environment if not provided via flags
	loadEnvIfEmpty(&rc.Token, bot.EnvGitHubToken)
	loadEnvIfEmpty(&rc.CommentBody, bot.EnvCommentBody)
	loadEnvIfEmpty(&rc.CommentID, bot.EnvCommentID)
	loadEnvIfEmpty(&rc.CommentAction, bot.EnvCommentAction)
	loadEnvIfEmpty(&rc.PRNumber, bot.EnvPRNumber)
	loadEnvIfEmpty(&rc.RepoOwner, bot.EnvRepoOwner)
	loadEnvIfEmpty(&rc.RepoName, bot.EnvRepoName)
	loadEnvIfEmpty(&rc.CommentAuthor, bot.EnvCommentAuthor)
	loadEnvIfEmpty(&rc.GitHubAppPrivateKey, bot.EnvGitHubAppPrivateKey)
	loadEnvIfEmpty(&rc.GitHubAppClientID, bot.EnvGitHubAppClientID)
	loadEnvIfEmpty(&rc.GitHubAppID, bot.EnvGitHubAppID)
	loadEnvIfEmpty(&rc.InstallationID, bot.EnvInstallationID)
	loadEnvIfEmpty(&rc.APIBaseURL, bot.EnvAPIBaseURL)

	// Load bot username with default for GitHub App
	loadEnvIfEmpty(&rc.BotUsername, bot.EnvBotUsername)
	if rc.BotUsername == "" {
		rc.BotUsername = bot.DefaultBotUsername
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
		return nil, bot.NewConfigError(bot.ErrConfigLoad, err)
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
