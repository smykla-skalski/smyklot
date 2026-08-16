package github

// repoConfigPath is where a repository's own Smyklot configuration lives
const repoConfigPath = ".github/smyklot.yaml"

// RepoConfig is a repository's own configuration file, as found.
//
// The path is carried alongside the bytes because the caller has to know which
// file it got: the format is decided by the name, and telling the decoder is
// what keeps a TOML syntax error from being reported as bad YAML.
type RepoConfig struct {
	// Path is the file the content came from, relative to the repository root.
	// Empty when the repository has no configuration file.
	Path string

	// Content is the file's decoded bytes, nil when no file was found.
	Content []byte

	// Superseded are the other paths that also hold a configuration file, in
	// search order. They are read by nothing and reported to the repository,
	// which is the point: a repository that migrated to TOML and left the
	// YAML behind has a file it believes is in charge and is not.
	Superseded []string
}

// Found reports whether the repository has a configuration file at all.
func (c RepoConfig) Found() bool { return c.Path != "" }

// Installation represents one installation of the GitHub App
type Installation struct {
	// ID identifies the installation, and is what an installation token is
	// minted for
	ID int64

	// AccountID is the immutable GitHub id of the installation owner.
	AccountID int64

	// Account is the login of the user or organization that installed the App
	Account string

	// AccountType is either Organization or User.
	AccountType string

	// AvatarURL is the public avatar of the installation owner.
	AvatarURL string

	// Permissions is what this installation has granted, keyed by GitHub's own
	// name for each - "administration", "issues", "contents" - against the level
	// granted.
	//
	// Read from the installation listing, which already carries it, so knowing
	// whether the App may do something costs no request. That matters because
	// the alternative is finding out by being refused: an installation that has
	// not approved a new permission is the ordinary state during a rollout, and
	// discovering it one 403 at a time would fill an operator's history with
	// failures that are really a question nobody has been asked yet.
	//
	// Empty means GitHub answered without the field. That is not a legitimate
	// "granted nothing" - the field is required on the installation object - so
	// it means the answer was malformed, and Grants says nothing was granted.
	Permissions map[string]string
}

// Permission levels that let Smyklot write, as GitHub spells them.
//
// Read is not among them and is not named: every other level answers the same
// way, so a constant for one of them would suggest a list that has to be kept
// complete.
const (
	PermissionWrite = "write"

	// PermissionAdmin is a level GitHub returns for a handful of permissions -
	// repository and organization projects among them - and never for the four
	// Smyklot reads, which are only ever read or write. It is accepted anyway,
	// because a permission added here later may use it and a silent false would
	// be the wrong direction to be wrong in.
	PermissionAdmin = "admin"
)

// Grants reports whether an installation may write through a permission.
//
// An installation whose permissions are unknown grants nothing. GitHub marks
// the field required, so its absence means a malformed or degraded answer
// rather than an installation that granted none - and proceeding on an answer
// that could not be read means writing to somebody's repositories on a guess.
// A 403 is a smaller problem than that.
func (i Installation) Grants(permission string) bool {
	switch i.Permissions[permission] {
	case PermissionWrite, PermissionAdmin:
		return true
	default:
		return false
	}
}

// Repository identifies a repository an installation can reach
type Repository struct {
	// ID is the immutable GitHub repository id.
	ID int64

	// Owner is the repository owner's login
	Owner string

	// Name is the repository name
	Name string

	// FullName is GitHub's canonical owner/name spelling.
	FullName string

	// Private reports whether repository contents require authentication.
	Private bool

	// DefaultBranch is the branch GitHub treats as the repository default.
	DefaultBranch string
}

// User is one immutable GitHub identity resolved by login.
type User struct {
	ID        int64
	Login     string
	Name      *string
	AvatarURL *string
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

// PullRequestState is the live state needed by background reconciliation.
// Unlike PRInfo it deliberately omits reviews and avoids that extra API call.
type PullRequestState struct {
	Number     int
	Open       bool
	Merged     bool
	Draft      bool
	HeadSHA    string
	BaseBranch string
	Labels     []string
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

	// ReactionPendingCIService fences a service-owned wait from the Action
	// runner without adding a second label to the pull request.
	ReactionPendingCIService ReactionType = "hooray"
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

	// LegacyLabelPendingCIServiceOwner is removed from pull requests created by
	// older service versions. New requests use only their method label.
	LegacyLabelPendingCIServiceOwner = "smyklot:pending:ci:service"

	// LabelPendingCIMerge indicates PR is waiting for CI before merge
	LabelPendingCIMerge = "smyklot:pending:ci"

	// LabelPendingCISquash indicates PR is waiting for CI before squash merge
	LabelPendingCISquash = "smyklot:pending:ci:squash"

	// LabelPendingCIRebase indicates PR is waiting for CI before rebase merge
	LabelPendingCIRebase = "smyklot:pending:ci:rebase"

	// LabelPendingCIMergeRequired indicates PR is waiting for required CI only before merge
	LabelPendingCIMergeRequired = "smyklot:pending:ci:required"

	// LabelPendingCISquashRequired indicates PR is waiting for required CI only before squash merge
	LabelPendingCISquashRequired = "smyklot:pending:ci:squash:required"

	// LabelPendingCIRebaseRequired indicates PR is waiting for required CI only before rebase merge
	LabelPendingCIRebaseRequired = "smyklot:pending:ci:rebase:required"

	// Legacy pending-CI labels remain readable during the organization migration.
	LegacyLabelPendingCIMerge          = "smyklot:pending-ci"
	LegacyLabelPendingCISquash         = "smyklot:pending-ci:squash"
	LegacyLabelPendingCIRebase         = "smyklot:pending-ci:rebase"
	LegacyLabelPendingCIMergeRequired  = "smyklot:pending-ci:required"
	LegacyLabelPendingCISquashRequired = "smyklot:pending-ci:squash:required"
	LegacyLabelPendingCIRebaseRequired = "smyklot:pending-ci:rebase:required"
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

// CIState is the exhaustive aggregate state of CI for one commit.
type CIState string

const (
	CIStatePassing       CIState = "passing"
	CIStatePending       CIState = "pending"
	CIStateFailing       CIState = "failing"
	CIStateNoChecks      CIState = "no_checks"
	CIStateIndeterminate CIState = "indeterminate"
)

// RequiredCheck identifies a branch-protection requirement. AppID is nil for
// legacy contexts that any status producer may satisfy.
type RequiredCheck struct {
	Context string
	AppID   *int64
}

// CheckStatus represents the aggregate state of Checks and commit statuses.
type CheckStatus struct {
	// State is the authoritative aggregate state.
	State CIState

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

	// Unknown is the number of contexts whose state Smyklot cannot classify.
	Unknown int

	// Missing is the number of required contexts not reported for this head.
	Missing int
}
