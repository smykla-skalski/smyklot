package orgsync

import "strings"

// RulesetConfig is the rulesets an installation expects its repositories to
// enforce.
//
// This is the whole stored document and the only type it decodes into, for the
// reason LabelConfig says: a second shape in the panel is how chunk 3's
// exclusions came to be stored and never read.
type RulesetConfig struct {
	Rulesets []Ruleset `json:"rulesets"`

	// AllowRemoval lets the planner propose deleting a ruleset configuration
	// does not name. Off by default, and even on it only ever proposes.
	//
	// It carries more weight here than it does for labels. The tool this
	// replaces had no delete path at all, so a ruleset dropped from
	// configuration went on enforcing for ever and a rename left two - but the
	// thing being removed is a rule somebody may be relying on to keep their
	// main branch, so it reaches a plan before it reaches GitHub.
	AllowRemoval bool `json:"allow_removal"`

	// Excludes are the ruleset names to leave alone entirely, neither written
	// nor removed. A repository that has hand-made a ruleset and wants to keep
	// it names it here.
	Excludes []string `json:"excludes,omitempty"`
}

// Exclusions is what the planner matches against.
func (c RulesetConfig) Exclusions() Excludes { return Excludes{Patterns: c.Excludes} }

// Ruleset is one ruleset as configuration describes it, whole.
//
// Values rather than pointers, unlike SettingsConfig, and the difference is the
// endpoint rather than a preference. Settings are patched field by field, so a
// field nobody configured has to be distinguishable from one configured off. A
// ruleset is written by replacement: the request defines the whole object and
// what it does not carry is not enforced. There is no request that means "leave
// this rule as it is", so a configuration that could express one would be
// promising something sync cannot do.
//
// Which is also why a ruleset sync owns is owned whole. The plan shows what a
// replacement would drop before it drops it.
type Ruleset struct {
	Name string `json:"name"`

	// Target is what the ruleset applies to: branch or tag.
	Target string `json:"target"`

	// Enforcement is whether the rules are applied, reported without being
	// applied, or held ready and switched off.
	Enforcement string `json:"enforcement"`

	Conditions   RulesetConditions    `json:"conditions"`
	BypassActors []RulesetBypassActor `json:"bypass_actors,omitempty"`
	Rules        RulesetRules         `json:"rules"`
}

// RulesetConditions is which refs the ruleset applies to.
type RulesetConditions struct {
	IncludeRefs []string `json:"include,omitempty"`
	ExcludeRefs []string `json:"exclude,omitempty"`
}

// bypassActorOrganizationAdmin is the one bypass actor type that names a role
// rather than somebody. GitHub reads it back with no id, so the comparison in
// rulesetplan.go leaves its id out.
const bypassActorOrganizationAdmin = "OrganizationAdmin"

// RulesetBypassActor is somebody who may step around the rules.
type RulesetBypassActor struct {
	ActorID   int64  `json:"actor_id"`
	ActorType string `json:"actor_type"`
	Mode      string `json:"bypass_mode"`
}

// RulesetRules is what a ruleset enforces.
type RulesetRules struct {
	Creation              bool `json:"creation,omitempty"`
	Deletion              bool `json:"deletion,omitempty"`
	NonFastForward        bool `json:"non_fast_forward,omitempty"`
	RequiredLinearHistory bool `json:"required_linear_history,omitempty"`
	RequiredSignatures    bool `json:"required_signatures,omitempty"`

	Update               *RulesetUpdateRule       `json:"update,omitempty"`
	PullRequest          *RulesetPullRequestRule  `json:"pull_request,omitempty"`
	RequiredStatusChecks *RulesetStatusChecksRule `json:"required_status_checks,omitempty"`
	CodeScanning         *RulesetCodeScanningRule `json:"code_scanning,omitempty"`
}

// RulesetUpdateRule restricts updates to a matching ref.
type RulesetUpdateRule struct {
	AllowsFetchAndMerge bool `json:"update_allows_fetch_and_merge,omitempty"`
}

// RulesetPullRequestRule is what a pull request must satisfy before it merges.
type RulesetPullRequestRule struct {
	RequiredApprovingReviewCount   int      `json:"required_approving_review_count,omitempty"`
	DismissStaleReviewsOnPush      bool     `json:"dismiss_stale_reviews_on_push,omitempty"`
	RequireCodeOwnerReview         bool     `json:"require_code_owner_review,omitempty"`
	RequireLastPushApproval        bool     `json:"require_last_push_approval,omitempty"`
	RequiredReviewThreadResolution bool     `json:"required_review_thread_resolution,omitempty"`
	AllowedMergeMethods            []string `json:"allowed_merge_methods"`
}

// RulesetStatusChecksRule is the checks that must pass.
type RulesetStatusChecksRule struct {
	Checks               []RulesetStatusCheck `json:"required_status_checks"`
	Strict               bool                 `json:"strict_required_status_checks_policy,omitempty"`
	DoNotEnforceOnCreate bool                 `json:"do_not_enforce_on_create,omitempty"`
}

// RulesetStatusCheck is one required check.
type RulesetStatusCheck struct {
	Context string `json:"context"`

	// IntegrationID pins the check to the App that reports it. Zero leaves it
	// unpinned, which is what somebody who has not thought about it wants.
	IntegrationID int64 `json:"integration_id,omitempty"`
}

// RulesetCodeScanningRule is the code-scanning thresholds that must hold.
type RulesetCodeScanningRule struct {
	Tools []RulesetCodeScanningTool `json:"code_scanning_tools"`
}

// RulesetCodeScanningTool is one tool and the thresholds it must meet.
type RulesetCodeScanningTool struct {
	Tool                    string `json:"tool"`
	AlertsThreshold         string `json:"alerts_threshold"`
	SecurityAlertsThreshold string `json:"security_alerts_threshold"`
}

// The values GitHub documents for a repository ruleset, as it spells them.
//
// Written out rather than passed through, which is what the tool this replaces
// did: `target`, `enforcement` and `bypass_mode` all reached the API untouched,
// so a typo became a 422 against somebody's repository rather than a message
// beside the field.
const (
	RulesetTargetBranch = "branch"
	RulesetTargetTag    = "tag"

	RulesetEnforcementActive   = "active"
	RulesetEnforcementEvaluate = "evaluate"
	RulesetEnforcementDisabled = "disabled"
)

// rulesetTargets is what a ruleset may apply to here.
//
// Push rulesets are refused rather than half-supported. They restrict file
// paths, extensions and sizes and share not one rule with the branch and tag
// rules above, so a push ruleset configured here would carry rules GitHub
// answers with a 422 - and this chunk exists to replace branch protection,
// which is a branch and a tag.
var rulesetTargets = map[string]string{
	RulesetTargetBranch: "refs/heads/",
	RulesetTargetTag:    "refs/tags/",
}

var rulesetEnforcements = map[string]bool{
	RulesetEnforcementActive:   true,
	RulesetEnforcementEvaluate: true,
	RulesetEnforcementDisabled: true,
}

// bypassActorTypes and bypassModes are GitHub's enumerations.
//
// `exempt` is here because GitHub documents it, not because the organization's
// own file happens to use it. It skips the rules and files no audit entry,
// which is a real third choice beside always and pull_request.
var (
	bypassActorTypes = map[string]bool{
		"Integration":                true,
		bypassActorOrganizationAdmin: true,
		"RepositoryRole":             true,
		"Team":                       true,
		"DeployKey":                  true,
	}

	bypassModes = map[string]bool{
		"always":       true,
		"pull_request": true,
		"exempt":       true,
	}
)

var mergeMethods = map[string]bool{"merge": true, "squash": true, "rebase": true}

var (
	alertsThresholds = map[string]bool{
		"none": true, "errors": true, "errors_and_warnings": true, "all": true,
	}

	securityAlertsThresholds = map[string]bool{
		"none": true, "critical": true, "high_or_higher": true,
		"medium_or_higher": true, "all": true,
	}
)

// refEverything and refDefaultBranch are the two ref conditions that are not
// patterns.
//
// The default-branch one earns its place in an organization-wide tool: the
// repositories being kept in step do not agree on what their default branch is
// called, and a configuration naming `refs/heads/main` protects nothing at all
// on the ones still calling it master.
const (
	refEverything     = "~ALL"
	refDefaultBranch  = "~DEFAULT_BRANCH"
	refNamePrefix     = "refs/"
	maxRulesetNameLen = 100
)

// Validate reports configuration GitHub would refuse, or would accept and
// silently do nothing with, at the point somebody writes it.
//
// The second half matters as much as the first here. An empty status-check list
// made the tool this replaces drop the rule with no log and no error, and a ref
// pattern aimed at the wrong kind of ref protects nothing while reading exactly
// like protection.
func (c RulesetConfig) Validate() error {
	if err := c.Exclusions().Validate(); err != nil {
		return err
	}

	seen := foldedNames{}

	for index, ruleset := range c.Rulesets {
		if err := ruleset.validate(index); err != nil {
			return err
		}

		// Name is the only handle sync has on a ruleset. GitHub permits two
		// with the same name, so nothing but this stops a configuration from
		// naming one the planner could not then tell apart from the other.
		// Folded, because two entries differing only in case are a distinction
		// nobody intends and a plan nobody can read.
		first, clashed := seen.clash(ruleset.Name)
		switch {
		case !clashed:
		case first == ruleset.Name:
			return invalid("ruleset %q is listed twice", first)
		default:
			return invalid("rulesets %q and %q differ only in case", first, ruleset.Name)
		}
	}

	return nil
}

func (r Ruleset) validate(index int) error {
	if err := validateName("ruleset", "name", index, r.Name, maxRulesetNameLen); err != nil {
		return err
	}

	prefix, known := rulesetTargets[r.Target]
	if !known {
		return invalid(
			"ruleset %q targets %q; this syncs %s and %s rulesets, and a push ruleset "+
				"restricts file paths and sizes rather than refs",
			r.Name, r.Target, RulesetTargetBranch, RulesetTargetTag)
	}

	if !rulesetEnforcements[r.Enforcement] {
		return invalid("ruleset %q is enforced %q, which is not %s, %s or %s",
			r.Name, r.Enforcement,
			RulesetEnforcementActive, RulesetEnforcementEvaluate, RulesetEnforcementDisabled)
	}

	if err := r.Conditions.validate(r.Name, r.Target, prefix); err != nil {
		return err
	}

	for _, actor := range r.BypassActors {
		if err := actor.validate(r.Name); err != nil {
			return err
		}
	}

	return r.Rules.validate(r.Name)
}

func (c RulesetConditions) validate(name, target, prefix string) error {
	// A ruleset that includes no ref matches no ref, and GitHub takes it
	// happily: it appears on the repository's rules page, reads as protection
	// everywhere, and enforces nothing on anything. The same silence every
	// other emptiness in this file is refused for.
	if len(c.IncludeRefs) == 0 {
		return invalid("ruleset %q covers no refs, so it would enforce nothing; "+
			"name a pattern, %s or %s", name, refEverything, refDefaultBranch)
	}

	for _, refs := range []struct {
		what     string
		patterns []string
	}{
		{"includes", c.IncludeRefs},
		{"excludes", c.ExcludeRefs},
	} {
		for _, pattern := range refs.patterns {
			if err := validateRefPattern(name, target, prefix, refs.what, pattern); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateRefPattern refuses a ref condition that would match nothing.
//
// The wrong-prefix case is the one worth the words. A `refs/tags/v*` pattern on
// a branch ruleset is accepted by GitHub and matches no ref that will ever
// exist, so the ruleset reads as protection in the panel, in the plan and on
// GitHub's own page, and protects nothing. That is the failure mode this whole
// chunk replaces - `matchBranchPattern` in the tool before it had the same
// shape, silently protecting nothing whenever its glob was wrong.
func validateRefPattern(name, target, prefix, what, pattern string) error {
	switch {
	// Every ref of whatever this ruleset targets, so it means something on
	// either one.
	case pattern == refEverything:
		return nil

	// A branch, and only ever a branch. A tag ruleset naming it is the
	// wrong-prefix case wearing a different hat: GitHub takes it, no tag is
	// ever the default branch, and the ruleset enforces nothing.
	case pattern == refDefaultBranch:
		if target != RulesetTargetBranch {
			return invalid("ruleset %q targets %s and %s %s, which no %s can ever be",
				name, target, what, refDefaultBranch, target)
		}

		return nil

	case strings.TrimSpace(pattern) == "":
		return invalid("ruleset %q %s an empty ref pattern", name, what)

	case !strings.HasPrefix(pattern, refNamePrefix):
		return invalid(
			"ruleset %q %s %q, which is not a ref: GitHub wants %s%s, %s or %s",
			name, what, pattern, prefix, "*", refEverything, refDefaultBranch)

	case !strings.HasPrefix(pattern, prefix):
		return invalid(
			"ruleset %q targets %s and %s %q, which no %s can ever match",
			name, target, what, pattern, target)
	}

	return nil
}

func (a RulesetBypassActor) validate(name string) error {
	switch {
	case a.ActorID <= 0:
		return invalid("ruleset %q has a bypass actor with no id", name)

	case !bypassActorTypes[a.ActorType]:
		return invalid("ruleset %q has a bypass actor of type %q, which GitHub does not know",
			name, a.ActorType)

	case !bypassModes[a.Mode]:
		return invalid("ruleset %q lets actor %d bypass %q, which is not always, "+
			"pull_request or exempt", name, a.ActorID, a.Mode)

	// GitHub says a deploy key cannot bypass on pull requests, because a deploy
	// key does not open one. The request is refused whole, so this is the whole
	// ruleset failing over an actor somebody added as an afterthought.
	case a.ActorType == "DeployKey" && a.Mode == "pull_request":
		return invalid("ruleset %q lets a deploy key bypass on pull requests, "+
			"which GitHub does not allow: a deploy key does not open one", name)
	}

	return nil
}

func (r RulesetRules) validate(name string) error {
	if pull := r.PullRequest; pull != nil {
		if err := pull.validate(name); err != nil {
			return err
		}
	}

	if checks := r.RequiredStatusChecks; checks != nil {
		if err := checks.validate(name); err != nil {
			return err
		}
	}

	if scanning := r.CodeScanning; scanning != nil {
		if err := scanning.validate(name); err != nil {
			return err
		}
	}

	return nil
}

func (r RulesetPullRequestRule) validate(name string) error {
	// No upper bound, because GitHub documents none. Guessing one would refuse
	// a configuration GitHub accepts, which is worse than the 422 it saves: the
	// plan shows a refusal, and a validation nobody can see refuses in silence.
	if r.RequiredApprovingReviewCount < 0 {
		return invalid("ruleset %q requires %d approving reviews",
			name, r.RequiredApprovingReviewCount)
	}

	if len(r.AllowedMergeMethods) == 0 {
		return invalid(
			"ruleset %q allows no way of merging a pull request; GitHub needs at least one "+
				"of merge, squash or rebase", name)
	}

	seen := make(map[string]bool, len(r.AllowedMergeMethods))

	for _, method := range r.AllowedMergeMethods {
		if !mergeMethods[method] {
			return invalid("ruleset %q allows merging by %q, which is not merge, "+
				"squash or rebase", name, method)
		}
		if seen[method] {
			return invalid("ruleset %q allows merging by %q twice", name, method)
		}
		seen[method] = true
	}

	return nil
}

// validate refuses a status-check rule with nothing in it.
//
// The tool this replaces dropped the whole rule when the list came out empty,
// with no log, no statistic and no error - and on an update it removed a rule
// that was already there. Its reason was sound, because the API does refuse an
// empty list; what it did with the reason was to make a repository quietly
// lose its required checks.
func (r RulesetStatusChecksRule) validate(name string) error {
	if len(r.Checks) == 0 {
		return invalid("ruleset %q requires status checks but names none, and GitHub "+
			"refuses an empty list", name)
	}

	seen := make(map[string]bool, len(r.Checks))

	for _, check := range r.Checks {
		trimmed := strings.TrimSpace(check.Context)
		switch {
		case trimmed == "":
			return invalid("ruleset %q requires a status check with no name", name)

		// Refused rather than trimmed, for the reason a padded name is: a check
		// is satisfied by a report arriving under exactly this string, and one
		// with a space on the end is a check nothing will ever report. Trimming
		// it silently would also let it sit beside its unpadded twin as a
		// second requirement neither of them is.
		case trimmed != check.Context:
			return invalid("ruleset %q requires the status check %q, which has "+
				"leading or trailing whitespace", name, check.Context)

		case seen[check.Context]:
			return invalid("ruleset %q requires the status check %q twice",
				name, check.Context)
		}

		seen[check.Context] = true
	}

	return nil
}

func (r RulesetCodeScanningRule) validate(name string) error {
	if len(r.Tools) == 0 {
		return invalid("ruleset %q requires code scanning but names no tool", name)
	}

	for _, tool := range r.Tools {
		switch {
		case strings.TrimSpace(tool.Tool) == "":
			return invalid("ruleset %q requires a code scanning tool with no name", name)

		case !alertsThresholds[tool.AlertsThreshold]:
			return invalid("ruleset %q sets the %s alert threshold to %q, "+
				"which GitHub does not know", name, tool.Tool, tool.AlertsThreshold)

		case !securityAlertsThresholds[tool.SecurityAlertsThreshold]:
			return invalid("ruleset %q sets the %s security alert threshold to %q, "+
				"which GitHub does not know", name, tool.Tool, tool.SecurityAlertsThreshold)
		}
	}

	return nil
}
