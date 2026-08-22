package github

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

// The eight contents GitHub accepts, spelled the way GitHub spells them. What
// a reaction means to the application putting it there is the application's
// word, not this package's.
const (
	// ReactionPlusOne is 👍
	ReactionPlusOne ReactionType = "+1"

	// ReactionMinusOne is 👎
	ReactionMinusOne ReactionType = "-1"

	// ReactionLaugh is 😄
	ReactionLaugh ReactionType = "laugh"

	// ReactionConfused is 😕
	ReactionConfused ReactionType = "confused"

	// ReactionHeart is ❤️
	ReactionHeart ReactionType = "heart"

	// ReactionHooray is 🎉
	ReactionHooray ReactionType = "hooray"

	// ReactionRocket is 🚀
	ReactionRocket ReactionType = "rocket"

	// ReactionEyes is 👀
	ReactionEyes ReactionType = "eyes"
)

// Reaction represents a reaction on a comment
type Reaction struct {
	// Type is the reaction type
	Type ReactionType

	// User is the username of the user who reacted
	User string
}

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
