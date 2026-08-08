package github

// repoConfigPath is where a repository's own Smyklot configuration lives
const repoConfigPath = ".github/smyklot.yaml"

// Installation represents one installation of the GitHub App
type Installation struct {
	// ID identifies the installation, and is what an installation token is
	// minted for
	ID int64

	// Account is the login of the user or organization that installed the App
	Account string
}

// Repository identifies a repository an installation can reach
type Repository struct {
	// Owner is the repository owner's login
	Owner string

	// Name is the repository name
	Name string
}

// MergeableState represents the merge state of a PR from GitHub REST API
type MergeableState string

const (
	// MergeableStateClean indicates PR can be merged
	MergeableStateClean MergeableState = "clean"

	// MergeableStateDirty indicates PR has conflicts
	MergeableStateDirty MergeableState = "dirty"

	// MergeableStateBlocked indicates PR is blocked by branch protection
	MergeableStateBlocked MergeableState = "blocked"

	// MergeableStateUnstable indicates PR has failing status checks
	MergeableStateUnstable MergeableState = "unstable"

	// MergeableStateUnknown indicates mergeability not yet computed
	MergeableStateUnknown MergeableState = "unknown"
)

// PRInfo contains information about a pull request
type PRInfo struct {
	// Number is the PR number
	Number int

	// State is the current state (open, closed, merged)
	State string

	// Mergeable indicates whether the PR can be merged (no conflicts)
	Mergeable bool

	// MergeableState provides detailed merge state (clean, dirty, blocked, unstable, unknown)
	MergeableState MergeableState

	// Author is the username of the PR author
	Author string

	// ApprovedBy contains usernames of approvers
	ApprovedBy []string

	// Title is the PR title
	Title string

	// Body is the PR description
	Body string

	// BaseBranch is the base branch (e.g. "main", "master")
	BaseBranch string
}

// ReactionType represents the type of emoji reaction
type ReactionType string

const (
	// ReactionSuccess represents success (✅)
	ReactionSuccess ReactionType = "+1"

	// ReactionError represents error (❌)
	ReactionError ReactionType = "-1"

	// ReactionWarning represents warning (⚠️)
	ReactionWarning ReactionType = "confused"

	// ReactionEyes represents acknowledgment (👀)
	ReactionEyes ReactionType = "eyes"

	// ReactionApprove represents approve command (👍)
	ReactionApprove ReactionType = "+1"

	// ReactionMerge represents merge command (🚀)
	ReactionMerge ReactionType = "rocket"

	// ReactionCleanup represents cleanup command (❤️)
	ReactionCleanup ReactionType = "heart"

	// ReactionPendingCI represents waiting for CI (👀)
	ReactionPendingCI ReactionType = "eyes"
)

// Reaction represents a reaction on a comment
type Reaction struct {
	// Type is the reaction type
	Type ReactionType

	// User is the username of the user who reacted
	User string
}

const (
	// LabelReactionApprove indicates PR was approved via 👍 reaction
	LabelReactionApprove = "smyklot:reaction-approve"

	// LabelReactionMerge indicates PR was merged via 🚀 reaction
	LabelReactionMerge = "smyklot:reaction-merge"

	// LabelReactionCleanup indicates cleanup was triggered via ❤️ reaction
	LabelReactionCleanup = "smyklot:reaction-cleanup"

	// LabelPendingCIMerge indicates PR is waiting for CI before merge
	LabelPendingCIMerge = "smyklot:pending-ci"

	// LabelPendingCISquash indicates PR is waiting for CI before squash merge
	LabelPendingCISquash = "smyklot:pending-ci:squash"

	// LabelPendingCIRebase indicates PR is waiting for CI before rebase merge
	LabelPendingCIRebase = "smyklot:pending-ci:rebase"

	// LabelPendingCIMergeRequired indicates PR is waiting for required CI only before merge
	LabelPendingCIMergeRequired = "smyklot:pending-ci:required"

	// LabelPendingCISquashRequired indicates PR is waiting for required CI only before squash merge
	LabelPendingCISquashRequired = "smyklot:pending-ci:squash:required"

	// LabelPendingCIRebaseRequired indicates PR is waiting for required CI only before rebase merge
	LabelPendingCIRebaseRequired = "smyklot:pending-ci:rebase:required"
)

// MergeMethod represents the type of merge method to use
type MergeMethod string

const (
	// MergeMethodMerge creates a merge commit
	MergeMethodMerge MergeMethod = "merge"

	// MergeMethodSquash squashes all commits into one
	MergeMethodSquash MergeMethod = "squash"

	// MergeMethodRebase rebases and merges
	MergeMethodRebase MergeMethod = "rebase"
)

// CheckStatus represents the status of CI checks on a commit
type CheckStatus struct {
	// AllPassing indicates all checks have completed successfully
	AllPassing bool

	// Pending indicates at least one check is still running
	Pending bool

	// Failing indicates at least one check has failed
	Failing bool

	// Summary provides a human-readable status (e.g., "5/6 checks passing")
	Summary string

	// Total is the total number of check runs
	Total int

	// Passed is the number of successful check runs
	Passed int

	// Failed is the number of failed check runs
	Failed int

	// InProgress is the number of check runs still running
	InProgress int
}
