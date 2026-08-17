package github

import (
	"context"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v90/github"
)

// RulesetSummary is a ruleset as the listing describes it.
//
// Its own type, because the listing is genuinely less than the thing. GitHub's
// index of rulesets carries the identity and nothing about what is enforced -
// no rules, no conditions, no bypass actors - so reading one as a whole ruleset
// would compare configuration against an object whose every rule is absent, and
// answer that every repository needs changing on every tick.
type RulesetSummary struct {
	ID          int64
	Name        string
	Target      string
	Enforcement string

	// Source is where the ruleset is defined: this repository, the
	// organization above it, or the enterprise above that.
	//
	// Read because an inherited ruleset is not this repository's to change and
	// not this repository's to delete, and because a repository-level ruleset
	// created beside one is a second set of rules enforced on the same refs.
	// GitHub applies every ruleset that matches and takes the strictest answer,
	// so the stack is invisible until somebody wonders why a merge is refused.
	Source RulesetSource
}

// RulesetSource is the level a ruleset is defined at, in GitHub's own spelling.
type RulesetSource string

const (
	RulesetSourceRepository   RulesetSource = "Repository"
	RulesetSourceOrganization RulesetSource = "Organization"
	RulesetSourceEnterprise   RulesetSource = "Enterprise"
)

// Inherited reports a ruleset defined above this repository.
func (s RulesetSource) Inherited() bool { return s != "" && s != RulesetSourceRepository }

// RepositoryRuleset is a whole ruleset: what it is called, where it applies,
// who may step around it and what it enforces.
//
// Values rather than pointers throughout, and that is the model rather than an
// oversight. A ruleset is written by replacement - the request defines the
// whole object and whatever it does not mention is not enforced - so there is
// no request that means "leave this rule as it is". A field this type does not
// carry is a field sync cannot preserve, which is why the set is chosen rather
// than partial: see RulesetRules.
type RepositoryRuleset struct {
	Name        string
	Target      string
	Enforcement string

	Conditions   RulesetConditions
	BypassActors []RulesetBypassActor
	Rules        RulesetRules

	// OtherRules names what GitHub is enforcing here that RulesetRules has no
	// field for, in GitHub's own spelling.
	//
	// Read on purpose, and the only part of this type that describes what
	// cannot be written. A ruleset is replaced whole, so a rule this version
	// does not model is a rule a replacement removes - and without this it
	// would be removed without ever having been read, so the plan somebody
	// approved could not have mentioned it. GitHub adds rule types faster than
	// anything here will follow, which makes that the ordinary case rather than
	// a corner of one.
	//
	// Empty for the overwhelming majority of rulesets. Never a value anything
	// branches on beyond "is there anything here"; it is words for a person.
	OtherRules []string
}

// RulesetConditions is which refs a ruleset applies to.
//
// Patterns, not branches. This is the whole reason branch protection is
// expressed as a ruleset here: `refs/heads/release/*` protects the release
// branch cut tomorrow, where the branch-protection endpoint takes one concrete
// branch and protects only what exists today.
type RulesetConditions struct {
	IncludeRefs []string
	ExcludeRefs []string
}

// RulesetBypassActor is somebody who may step around the rules.
type RulesetBypassActor struct {
	ActorID   int64
	ActorType string

	// Mode is when the bypass applies: always, only on pull requests, or
	// exempt, which also files no audit entry.
	Mode string
}

// RulesetRules is what a ruleset enforces.
//
// The branch and tag rules, which is what this replaces branch protection with.
// Push rulesets restrict file paths, extensions and sizes instead and share no
// rule with these, so they are refused at configuration rather than half-built
// here.
type RulesetRules struct {
	Creation              bool
	Deletion              bool
	NonFastForward        bool
	RequiredLinearHistory bool
	RequiredSignatures    bool

	// Update, unlike its neighbours, carries a parameter, so it is a pointer:
	// absent is "ref updates are not restricted" and present is a restriction
	// that either does or does not permit fetch-and-merge.
	Update *RulesetUpdateRule

	PullRequest          *RulesetPullRequestRule
	RequiredStatusChecks *RulesetStatusChecksRule
	CodeScanning         *RulesetCodeScanningRule
}

// RulesetUpdateRule restricts updates to a matching ref.
type RulesetUpdateRule struct {
	AllowsFetchAndMerge bool
}

// RulesetPullRequestRule is what a pull request must satisfy before it merges.
type RulesetPullRequestRule struct {
	RequiredApprovingReviewCount   int
	DismissStaleReviewsOnPush      bool
	RequireCodeOwnerReview         bool
	RequireLastPushApproval        bool
	RequiredReviewThreadResolution bool
	AllowedMergeMethods            []string
}

// RulesetStatusChecksRule is the checks that must pass.
type RulesetStatusChecksRule struct {
	Checks []RulesetStatusCheck
	Strict bool

	// DoNotEnforceOnCreate lets a branch created from a protected one exist
	// before its checks have ever run.
	DoNotEnforceOnCreate bool
}

// RulesetStatusCheck is one required check.
type RulesetStatusCheck struct {
	Context string

	// IntegrationID pins the check to the App that reports it, so a second App
	// posting the same context cannot satisfy it. Zero leaves it unpinned.
	IntegrationID int64
}

// RulesetCodeScanningRule is the code-scanning thresholds that must hold.
type RulesetCodeScanningRule struct {
	Tools []RulesetCodeScanningTool
}

// RulesetCodeScanningTool is one tool and the thresholds it must meet.
type RulesetCodeScanningTool struct {
	Tool                    string
	AlertsThreshold         string
	SecurityAlertsThreshold string
}

// ListRepositoryRulesets reads every ruleset that applies to a repository,
// including the ones it inherits.
//
// Paginated. GitHub's default page is thirty, and past it the tool this
// replaces missed entries entirely - so a ruleset it already manages read as
// absent, it created a second one with the same name, which GitHub permits, and
// from then on it updated whichever of the two came back first.
//
// Parents included on purpose. They are not this repository's to write, and
// that is exactly why they have to be read: a repository-level ruleset created
// beside an inherited one of the same name is two sets of rules enforced at
// once, and neither the panel nor the person reading the plan would see it.
func (c *Client) ListRepositoryRulesets(
	ctx context.Context,
	owner, repo string,
) ([]RulesetSummary, error) {
	path := fmt.Sprintf("/repos/%s/%s/rulesets", owner, repo)

	raw, err := paginate(ctx, path,
		func(ctx context.Context, opts *gogithub.ListOptions) (
			[]*gogithub.RepositoryRuleset, *gogithub.Response, error,
		) {
			return c.gh.Repositories.GetAllRulesets(ctx, owner, repo,
				&gogithub.RepositoryListRulesetsOptions{
					IncludesParents: gogithub.Ptr(true),
					ListOptions:     *opts,
				})
		})
	if err != nil {
		return nil, err
	}

	rulesets := make([]RulesetSummary, 0, len(raw))
	for _, ruleset := range raw {
		rulesets = append(rulesets, RulesetSummary{
			ID:          ruleset.GetID(),
			Name:        ruleset.GetName(),
			Target:      string(targetOf(ruleset)),
			Enforcement: string(ruleset.GetEnforcement()),
			Source:      RulesetSource(sourceTypeOf(ruleset)),
		})
	}

	return rulesets, nil
}

// sourceTypeOf reads the level a listed ruleset came from.
//
// Absent reads as the repository's own, which is the safe direction: the value
// decides whether sync may write to a ruleset, and treating an unlabelled one
// as inherited would leave a repository's own ruleset unmanageable for ever
// with nothing to say why.
func sourceTypeOf(ruleset *gogithub.RepositoryRuleset) gogithub.RulesetSourceType {
	if source := ruleset.GetSourceType(); source != nil {
		return *source
	}

	return gogithub.RulesetSourceTypeRepository
}

// targetOf reads what a ruleset applies to. Absent stays absent: unlike the
// source, an unnamed target is nothing to guess at, and the planner compares it
// rather than branching on it.
func targetOf(ruleset *gogithub.RepositoryRuleset) gogithub.RulesetTarget {
	if target := ruleset.GetTarget(); target != nil {
		return *target
	}

	return ""
}

// GetRepositoryRuleset reads one ruleset whole.
//
// A second request per ruleset, because the listing carries no rules. That cost
// is why the planner asks only about the rulesets configuration names: the
// alternative is one request per ruleset a repository happens to have, which
// somebody else decides the size of.
func (c *Client) GetRepositoryRuleset(
	ctx context.Context,
	owner, repo string,
	id int64,
) (RepositoryRuleset, error) {
	path := fmt.Sprintf("/repos/%s/%s/rulesets/%d", owner, repo, id)

	raw, _, err := c.gh.Repositories.GetRuleset(ctx, owner, repo, id, false)
	if err != nil {
		return RepositoryRuleset{}, wrapError(ErrAPIRequest, http.MethodGet, path, err)
	}

	return asRepositoryRuleset(raw), nil
}

// CreateRepositoryRuleset adds a ruleset to a repository.
func (c *Client) CreateRepositoryRuleset(
	ctx context.Context,
	owner, repo string,
	ruleset RepositoryRuleset,
) error {
	path := fmt.Sprintf("/repos/%s/%s/rulesets", owner, repo)

	_, _, err := c.gh.Repositories.CreateRuleset(ctx, owner, repo, asGitHubRuleset(ruleset))

	return wrapError(ErrAPIRequest, http.MethodPost, path, err)
}

// UpdateRepositoryRuleset replaces a ruleset a repository already has.
//
// A replacement rather than a patch, because that is what the endpoint is: what
// the request does not carry stops being enforced. Sync owns a ruleset whole or
// not at all, and the plan shows what the replacement drops before it runs.
func (c *Client) UpdateRepositoryRuleset(
	ctx context.Context,
	owner, repo string,
	id int64,
	ruleset RepositoryRuleset,
) error {
	path := fmt.Sprintf("/repos/%s/%s/rulesets/%d", owner, repo, id)

	_, _, err := c.gh.Repositories.UpdateRuleset(ctx, owner, repo, id, asGitHubRuleset(ruleset))

	return wrapError(ErrAPIRequest, http.MethodPut, path, err)
}

// DeleteRepositoryRuleset removes a ruleset from a repository.
//
// Only ever called for a ruleset defined on the repository itself. An inherited
// one cannot be deleted through this endpoint, and a request that tried would
// report a repository failing at something nobody asked for.
func (c *Client) DeleteRepositoryRuleset(
	ctx context.Context,
	owner, repo string,
	id int64,
) error {
	path := fmt.Sprintf("/repos/%s/%s/rulesets/%d", owner, repo, id)

	_, err := c.gh.Repositories.DeleteRuleset(ctx, owner, repo, id)

	return wrapError(ErrAPIRequest, http.MethodDelete, path, err)
}

// asRepositoryRuleset reads what GitHub answered into what sync compares.
func asRepositoryRuleset(raw *gogithub.RepositoryRuleset) RepositoryRuleset {
	if raw == nil {
		return RepositoryRuleset{}
	}

	ruleset := RepositoryRuleset{
		Name:        raw.GetName(),
		Target:      string(targetOf(raw)),
		Enforcement: string(raw.GetEnforcement()),
	}

	if refs := raw.GetConditions().GetRefName(); refs != nil {
		ruleset.Conditions = RulesetConditions{
			IncludeRefs: refs.Include,
			ExcludeRefs: refs.Exclude,
		}
	}

	for _, actor := range raw.GetBypassActors() {
		ruleset.BypassActors = append(ruleset.BypassActors, RulesetBypassActor{
			ActorID:   actor.GetActorID(),
			ActorType: string(actorTypeOf(actor)),
			Mode:      string(bypassModeOf(actor)),
		})
	}

	ruleset.Rules = asRulesetRules(raw.GetRules())
	ruleset.OtherRules = unmodelledRules(raw.GetRules())

	return ruleset
}

// unmodelledRules names every rule GitHub reported that RulesetRules cannot
// carry, in GitHub's own spelling and in a fixed order.
//
// Written out rather than derived, because there is nothing to derive it from:
// the shape is a struct of typed pointers, and a rule type added to it upstream
// has to be either modelled or named here. That is the point - the list is
// where somebody notices.
//
// The push and repository rules cannot appear on a branch or tag ruleset, which
// is the only kind sync writes. They are here anyway, because a ruleset read is
// whatever GitHub answers with and a silent omission is the failure this whole
// function exists to prevent.
func unmodelledRules(rules *gogithub.RepositoryRulesetRules) []string {
	if rules == nil {
		return nil
	}

	var found []string

	for _, rule := range []struct {
		present bool
		name    string
	}{
		{rules.MergeQueue != nil, "merge_queue"},
		{rules.RequiredDeployments != nil, "required_deployments"},
		{rules.CommitMessagePattern != nil, "commit_message_pattern"},
		{rules.CommitAuthorEmailPattern != nil, "commit_author_email_pattern"},
		{rules.CommitterEmailPattern != nil, "committer_email_pattern"},
		{rules.BranchNamePattern != nil, "branch_name_pattern"},
		{rules.TagNamePattern != nil, "tag_name_pattern"},
		{rules.Workflows != nil, "workflows"},
		{rules.CopilotCodeReview != nil, "copilot_code_review"},

		{rules.FileExtensionRestriction != nil, "file_extension_restriction"},
		{rules.FilePathRestriction != nil, "file_path_restriction"},
		{rules.MaxFilePathLength != nil, "max_file_path_length"},
		{rules.MaxFileSize != nil, "max_file_size"},

		{rules.RepositoryCreate != nil, "repository_create"},
		{rules.RepositoryDelete != nil, "repository_delete"},
		{rules.RepositoryName != nil, "repository_name"},
		{rules.RepositoryTransfer != nil, "repository_transfer"},
		{rules.RepositoryVisibility != nil, "repository_visibility"},

		// Not a rule of its own but a parameter of one that is modelled, and
		// dropped by a replacement exactly the same way. Named beside them
		// because what matters to a person reading the plan is that it goes.
		{len(rules.PullRequest.GetRequiredReviewers()) > 0,
			"pull_request.required_reviewers"},
	} {
		if rule.present {
			found = append(found, rule.name)
		}
	}

	return found
}

func actorTypeOf(actor *gogithub.BypassActor) gogithub.BypassActorType {
	if kind := actor.GetActorType(); kind != nil {
		return *kind
	}

	return ""
}

func bypassModeOf(actor *gogithub.BypassActor) gogithub.BypassMode {
	if mode := actor.GetBypassMode(); mode != nil {
		return *mode
	}

	return ""
}

func asRulesetRules(raw *gogithub.RepositoryRulesetRules) RulesetRules {
	if raw == nil {
		return RulesetRules{}
	}

	rules := RulesetRules{
		Creation:              raw.Creation != nil,
		Deletion:              raw.Deletion != nil,
		NonFastForward:        raw.NonFastForward != nil,
		RequiredLinearHistory: raw.RequiredLinearHistory != nil,
		RequiredSignatures:    raw.RequiredSignatures != nil,
	}

	if raw.Update != nil {
		rules.Update = &RulesetUpdateRule{
			AllowsFetchAndMerge: raw.Update.UpdateAllowsFetchAndMerge,
		}
	}

	if pull := raw.PullRequest; pull != nil {
		rule := &RulesetPullRequestRule{
			RequiredApprovingReviewCount:   pull.RequiredApprovingReviewCount,
			DismissStaleReviewsOnPush:      pull.DismissStaleReviewsOnPush,
			RequireCodeOwnerReview:         pull.RequireCodeOwnerReview,
			RequireLastPushApproval:        pull.RequireLastPushApproval,
			RequiredReviewThreadResolution: pull.RequiredReviewThreadResolution,
		}

		for _, method := range pull.AllowedMergeMethods {
			rule.AllowedMergeMethods = append(rule.AllowedMergeMethods, string(method))
		}

		rules.PullRequest = rule
	}

	if checks := raw.RequiredStatusChecks; checks != nil {
		rule := &RulesetStatusChecksRule{
			Strict:               checks.StrictRequiredStatusChecksPolicy,
			DoNotEnforceOnCreate: checks.GetDoNotEnforceOnCreate(),
		}

		for _, check := range checks.RequiredStatusChecks {
			rule.Checks = append(rule.Checks, RulesetStatusCheck{
				Context:       check.Context,
				IntegrationID: check.GetIntegrationID(),
			})
		}

		rules.RequiredStatusChecks = rule
	}

	if scanning := raw.CodeScanning; scanning != nil {
		rule := &RulesetCodeScanningRule{}

		for _, tool := range scanning.CodeScanningTools {
			rule.Tools = append(rule.Tools, RulesetCodeScanningTool{
				Tool:                    tool.Tool,
				AlertsThreshold:         string(tool.AlertsThreshold),
				SecurityAlertsThreshold: string(tool.SecurityAlertsThreshold),
			})
		}

		rules.CodeScanning = rule
	}

	return rules
}

// asGitHubRuleset renders what sync decided into what the endpoint takes.
func asGitHubRuleset(ruleset RepositoryRuleset) gogithub.RepositoryRuleset {
	target := gogithub.RulesetTarget(ruleset.Target)

	raw := gogithub.RepositoryRuleset{
		Name:        ruleset.Name,
		Target:      &target,
		Enforcement: gogithub.RulesetEnforcement(ruleset.Enforcement),

		// Always sent, and never null. GitHub refuses a null where it expects a
		// list, and an unconditioned ruleset - one that matches every ref - is
		// spelled with empty lists rather than by leaving the object out.
		Conditions: &gogithub.RepositoryRulesetConditions{
			RefName: &gogithub.RepositoryRulesetRefConditionParameters{
				Include: orEmpty(ruleset.Conditions.IncludeRefs),
				Exclude: orEmpty(ruleset.Conditions.ExcludeRefs),
			},
		},

		BypassActors: make([]*gogithub.BypassActor, 0, len(ruleset.BypassActors)),
		Rules:        asGitHubRules(ruleset.Rules),
	}

	for _, actor := range ruleset.BypassActors {
		actorType := gogithub.BypassActorType(actor.ActorType)
		mode := gogithub.BypassMode(actor.Mode)

		raw.BypassActors = append(raw.BypassActors, &gogithub.BypassActor{
			ActorID:    gogithub.Ptr(actor.ActorID),
			ActorType:  &actorType,
			BypassMode: &mode,
		})
	}

	return raw
}

func asGitHubRules(rules RulesetRules) *gogithub.RepositoryRulesetRules {
	raw := &gogithub.RepositoryRulesetRules{}

	if rules.Creation {
		raw.Creation = &gogithub.EmptyRuleParameters{}
	}
	if rules.Deletion {
		raw.Deletion = &gogithub.EmptyRuleParameters{}
	}
	if rules.NonFastForward {
		raw.NonFastForward = &gogithub.EmptyRuleParameters{}
	}
	if rules.RequiredLinearHistory {
		raw.RequiredLinearHistory = &gogithub.EmptyRuleParameters{}
	}
	if rules.RequiredSignatures {
		raw.RequiredSignatures = &gogithub.EmptyRuleParameters{}
	}

	if rules.Update != nil {
		raw.Update = &gogithub.UpdateRuleParameters{
			UpdateAllowsFetchAndMerge: rules.Update.AllowsFetchAndMerge,
		}
	}

	if pull := rules.PullRequest; pull != nil {
		rule := &gogithub.PullRequestRuleParameters{
			RequiredApprovingReviewCount:   pull.RequiredApprovingReviewCount,
			DismissStaleReviewsOnPush:      pull.DismissStaleReviewsOnPush,
			RequireCodeOwnerReview:         pull.RequireCodeOwnerReview,
			RequireLastPushApproval:        pull.RequireLastPushApproval,
			RequiredReviewThreadResolution: pull.RequiredReviewThreadResolution,
		}

		for _, method := range pull.AllowedMergeMethods {
			rule.AllowedMergeMethods = append(
				rule.AllowedMergeMethods, gogithub.PullRequestMergeMethod(method))
		}

		raw.PullRequest = rule
	}

	if checks := rules.RequiredStatusChecks; checks != nil {
		rule := &gogithub.RequiredStatusChecksRuleParameters{
			StrictRequiredStatusChecksPolicy: checks.Strict,
			DoNotEnforceOnCreate:             gogithub.Ptr(checks.DoNotEnforceOnCreate),
			RequiredStatusChecks: make(
				[]*gogithub.RuleStatusCheck, 0, len(checks.Checks)),
		}

		for _, check := range checks.Checks {
			status := &gogithub.RuleStatusCheck{Context: check.Context}
			if check.IntegrationID != 0 {
				status.IntegrationID = gogithub.Ptr(check.IntegrationID)
			}

			rule.RequiredStatusChecks = append(rule.RequiredStatusChecks, status)
		}

		raw.RequiredStatusChecks = rule
	}

	if scanning := rules.CodeScanning; scanning != nil {
		rule := &gogithub.CodeScanningRuleParameters{
			CodeScanningTools: make(
				[]*gogithub.RuleCodeScanningTool, 0, len(scanning.Tools)),
		}

		for _, tool := range scanning.Tools {
			rule.CodeScanningTools = append(rule.CodeScanningTools,
				&gogithub.RuleCodeScanningTool{
					Tool: tool.Tool,
					AlertsThreshold: gogithub.CodeScanningAlertsThreshold(
						tool.AlertsThreshold),
					SecurityAlertsThreshold: gogithub.CodeScanningSecurityAlertsThreshold(
						tool.SecurityAlertsThreshold),
				})
		}

		raw.CodeScanning = rule
	}

	return raw
}

// orEmpty turns a nil slice into an empty one, because the two are the same
// instruction here and only one of them is legal JSON to GitHub.
func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}
