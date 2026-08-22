package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
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
	rc := &bot.RuntimeConfig{}
	loadEnvIfEmpty(&rc.Token, bot.EnvGitHubToken)
	loadEnvIfEmpty(&rc.GitHubAppPrivateKey, bot.EnvGitHubAppPrivateKey)
	loadEnvIfEmpty(&rc.GitHubAppClientID, bot.EnvGitHubAppClientID)
	loadEnvIfEmpty(&rc.GitHubAppID, bot.EnvGitHubAppID)
	loadEnvIfEmpty(&rc.InstallationID, bot.EnvInstallationID)
	loadEnvIfEmpty(&rc.BotUsername, bot.EnvBotUsername)
	loadEnvIfEmpty(&rc.APIBaseURL, bot.EnvAPIBaseURL)

	if rc.BotUsername == "" {
		rc.BotUsername = bot.DefaultBotUsername
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
	repoOwner, repoName, err := bot.ParseRepo(repo)
	if err != nil {
		return err
	}

	client, err := github.NewClient(token, rc.APIBaseURL)
	if err != nil {
		return bot.NewGitHubError(bot.ErrGitHubClient, err)
	}

	// Layer the repository's own configuration over the workflow's, the same
	// way a comment does. A sweep that ignored the file would act on reactions
	// with settings the repository had turned off, and would keep sweeping a
	// repository that has moved to the service
	bc, err = effectiveConfig(ctx, client, repoOwner, repoName, bc)
	if err != nil {
		return bot.ReportUnusableRepoConfig(ctx, err)
	}

	// Decided before CODEOWNERS is read, the way the service's sweep decides
	// it. A repository on the service runs this workflow every five minutes,
	// and reading a file it will not use is the whole cost of that tick
	if bot.ActionStandsDown(ctx, bc) {
		return nil
	}

	checker, err := bot.NewPermissionChecker(ctx, client, repoOwner, repoName)
	if err != nil {
		return err
	}

	// Poll and process all open PRs
	return bot.PollAllPRs(
		ctx, client, checker, bc, repoOwner, repoName, rc.BotUsername,
		bot.CommandEnvironment{}, true,
	)
}

// getPollConfig retrieves repo and token from flags or environment
func getPollConfig(cmd *cobra.Command, rc *bot.RuntimeConfig) (string, string, error) {
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
		owner := os.Getenv(bot.EnvRepoOwner)
		name := os.Getenv(bot.EnvRepoName)
		if owner == "" || name == "" {
			return "", "", fmt.Errorf("repository not specified (use --repo or REPO_OWNER/REPO_NAME env vars)")
		}
		repo = bot.RepoFullName(owner, name)
	}

	// Get token from environment if not provided via flag
	if token == "" {
		token = os.Getenv(bot.EnvGitHubToken)
		if token == "" {
			// Try GitHub App auth
			installationToken, err := bot.InstallationToken(rc)
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
