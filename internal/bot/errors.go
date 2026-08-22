package bot

import (
	"errors"
	"fmt"
)

// Sentinel errors for common failure cases.
var (
	// errMissingEnvVar is returned when a required environment variable is missing
	errMissingEnvVar = errors.New("required environment variable is missing")

	// ErrInvalidInput is returned when invalid input is provided
	ErrInvalidInput = errors.New("invalid input provided")

	// ErrGitHubClient is returned when GitHub client creation fails
	ErrGitHubClient = errors.New("GitHub client error")

	// ErrGitHubAppAuth is returned when GitHub App authentication fails
	ErrGitHubAppAuth = errors.New("GitHub App authentication failed")

	// ErrPermissionCheck is returned when the permission check fails
	ErrPermissionCheck = errors.New("permission check failed")

	// errPostComment is returned when posting a comment fails
	errPostComment = errors.New("failed to post comment")

	// errAddReaction is returned when adding a reaction fails
	errAddReaction = errors.New("failed to add reaction")

	// errApprovePR is returned when approving a PR fails
	errApprovePR = errors.New("failed to approve PR")

	// errMergePR is returned when merging a PR fails
	errMergePR = errors.New("failed to merge PR")

	// errGetCodeowners is returned when fetching CODEOWNERS from GitHub fails
	errGetCodeowners = errors.New("failed to fetch CODEOWNERS from GitHub")

	// errInitPermissions is returned when initializing permissions fails
	errInitPermissions = errors.New("failed to initialize permissions")

	// errStepSummary is returned when step summary operations fail
	errStepSummary = errors.New("failed to write step summary")

	// ErrConfigLoad is returned when loading configuration fails
	ErrConfigLoad = errors.New("failed to load configuration")

	// ErrRepoConfigInvalid is returned when a repository's own configuration
	// file exists but cannot be parsed. Unlike a failure to fetch it, retrying
	// will not help, so the repository is told rather than left guessing
	ErrRepoConfigInvalid = errors.New("repository configuration file is invalid")

	// ErrGetPRs is returned when fetching open PRs from GitHub fails
	ErrGetPRs = errors.New("failed to fetch open PRs from GitHub")

	// ErrListInstallations is returned when listing the App's installations fails
	ErrListInstallations = errors.New("failed to list App installations")

	// ErrListRepos is returned when listing an installation's repositories fails
	ErrListRepos = errors.New("failed to list installation repositories")

	ErrRequiredWorkflowsUnsupported = errors.New(
		"required-only merge-after-CI does not support required workflow rules",
	)
)

// inputError represents an error with parsing or validating input.
type inputError struct {
	Op      error
	Input   string
	Details string
}

func NewInputError(op error, input, details string) error {
	return &inputError{
		Op:      op,
		Input:   input,
		Details: details,
	}
}

func (e *inputError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Op, e.Input, e.Details)
	}

	return fmt.Sprintf("%s: %s", e.Op, e.Input)
}

func (e *inputError) Unwrap() error {
	return e.Op
}

func (e *inputError) Is(target error) bool {
	return errors.Is(e.Op, target)
}

// opError names the operation that failed and carries the cause underneath it.
type opError struct {
	Op  error
	Err error
}

func NewGitHubError(op, err error) error {
	return &opError{Op: op, Err: err}
}

func NewConfigError(op, err error) error {
	return &opError{Op: op, Err: err}
}

func (e *opError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Op, e.Err)
	}

	return e.Op.Error()
}

func (e *opError) Unwrap() error {
	if e.Err != nil {
		return e.Err
	}

	return e.Op
}

// Is matches on the operation, since Unwrap walks to the cause instead.
//
// Without this a caller cannot tell one failure from another - errors.Is would
// follow Unwrap past Op and never see it.
func (e *opError) Is(target error) bool {
	return errors.Is(e.Op, target)
}
