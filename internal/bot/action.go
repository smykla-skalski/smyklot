package bot

import (
	"context"
	"strconv"
	"strings"
	"text/template"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/githubapp"
	"github.com/smykla-skalski/smyklot/pkg/logging"
	"github.com/smykla-skalski/smyklot/pkg/permissions"
)

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
	AllowDraftMerges       bool
	AllowedCommands        string
	CommandAliases         map[string]string
}

// CheckUserPermission checks if a user has permission to approve/merge
//
// It first checks CODEOWNERS permissions. If no CODEOWNERS exists (empty),
// it falls back to checking if the user has admin/write repository permissions.
func CheckUserPermission(
	ctx context.Context,
	client *github.Client,
	checker *permissions.Checker,
	username, owner, repo string,
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

// InstallationToken generates a GitHub App installation token if credentials are provided.
//
// Returns an empty string if GitHub App credentials are not configured.
// Returns the token on success.
func InstallationToken(rc *RuntimeConfig) (string, error) {
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

// WriteStepSummary writes the effective configuration to GitHub Actions step summary.
func WriteStepSummary(rc *RuntimeConfig, bc *config.Config) error {
	// Rendered before anything is opened, so appendStepSummary stays the one
	// place that knows how to write to the summary
	var rendered strings.Builder

	tmpl, err := template.New(summaryTemplateName).Parse(stepSummaryTemplate)
	if err != nil {
		return NewGitHubError(errStepSummary, err)
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
		AllowDraftMerges:       bc.AllowDraftMerges,
		AllowedCommands:        allowedCommands,
		CommandAliases:         bc.CommandAliases,
	}

	if err := tmpl.Execute(&rendered, data); err != nil {
		return NewGitHubError(errStepSummary, err)
	}

	return appendStepSummary(rendered.String())
}
