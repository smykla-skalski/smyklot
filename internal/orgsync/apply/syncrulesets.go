package apply

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// errSyncRulesetUnaddressed is an action that says to change a ruleset without
// saying which one.
//
// It cannot come from a plan this version computed. Refused rather than
// resolved by name here, because resolving it would write to whatever holds
// that name at apply time - which, on a repository that has since grown a
// second ruleset of the name, is a coin toss nobody approved.
var errSyncRulesetUnaddressed = errors.New("the plan does not say which ruleset to change")

// errSyncRulesetTaken is a name that was free when the plan was computed and is
// not any more. The action fails, the next reconcile reads what is there, and
// what happens then depends on what it finds - one ruleset it can manage, or
// two it cannot.
var errSyncRulesetTaken = errors.New("a ruleset of that name already exists")

// readRulesets answers what a repository has, at the two levels of detail the
// planner needs.
//
// One request lists everything that applies, including what the repository
// inherits. Only the rulesets configuration names are then read whole, because
// the listing carries no rules and reading the rest would cost one request per
// ruleset somebody else decides the number of.
//
// A surplus ruleset therefore reaches the plan by name alone. That is enough to
// remove one, and the plan says so in as many words rather than printing an
// empty ruleset that would read as one enforcing nothing.
func readRulesets(
	ctx context.Context,
	client *github.Client,
	owner, name string,
	config orgsync.RulesetConfig,
) ([]orgsync.CurrentRuleset, error) {
	listed, err := client.ListRepositoryRulesets(ctx, owner, name)
	if err != nil {
		return nil, err
	}

	configured := make(map[string]bool, len(config.Rulesets))
	for _, ruleset := range config.Rulesets {
		configured[strings.ToLower(ruleset.Name)] = true
	}

	current := make([]orgsync.CurrentRuleset, 0, len(listed))

	for _, summary := range listed {
		found := orgsync.CurrentRuleset{
			ID:        summary.ID,
			Name:      summary.Name,
			Inherited: summary.Source.Inherited(),
		}

		if !found.Inherited && configured[strings.ToLower(summary.Name)] {
			whole, err := client.GetRepositoryRuleset(ctx, owner, name, summary.ID)
			if err != nil {
				return nil, err
			}

			defined := asConfiguredRuleset(whole)
			found.Defined = &defined
			found.Unmanaged = whole.OtherRules
		}

		current = append(current, found)
	}

	return current, nil
}

// applyRulesetAction performs one ruleset change.
//
// Everything it needs is on the action, including the id, because that is what
// somebody read and approved. Looking the ruleset up again by name would find
// whatever holds the name now.
func applyRulesetAction(
	ctx context.Context,
	client *github.Client,
	owner, name string,
	action orgsync.Action,
) error {
	if len(action.Payload) == 0 {
		return fmt.Errorf("%w: %s %q",
			errSyncPayloadMissing, action.Operation, action.Subject)
	}

	resolved, err := orgsync.DecodeRulesetAction(action.Payload)
	if err != nil {
		return err
	}

	switch action.Operation {
	case orgsync.OperationCreate:
		return createRuleset(ctx, client, owner, name, resolved.Ruleset)

	case orgsync.OperationUpdate:
		if resolved.ID == 0 {
			return fmt.Errorf("%w: %q", errSyncRulesetUnaddressed, action.Subject)
		}

		return client.UpdateRepositoryRuleset(
			ctx, owner, name, resolved.ID, asClientRuleset(resolved.Ruleset))

	case orgsync.OperationDelete:
		if resolved.ID == 0 {
			return fmt.Errorf("%w: %q", errSyncRulesetUnaddressed, action.Subject)
		}

		return client.DeleteRepositoryRuleset(ctx, owner, name, resolved.ID)

	default:
		return fmt.Errorf("%w: %s", errSyncOperationUnknown, action.Operation)
	}
}

// createRuleset adds a ruleset, unless the repository has grown one of that
// name since the plan was computed.
//
// The one place a re-read is right. Everything else about an action is decided
// at plan time on purpose, but a create carries no id - there is nothing to
// carry - and GitHub permits two rulesets with one name, so a name claimed
// between approval and apply would be answered with a second copy rather than
// a refusal. Two of a name is the state nothing downstream can address, which
// is the failure this whole chunk indicts the tool before it for.
//
// It does not close the window; nothing can, because GitHub offers no
// conditional create. It narrows it from however long a plan waits for somebody
// to the time between these two requests, and it turns the outcome from a
// silent duplicate into a failed action that says why.
func createRuleset(
	ctx context.Context,
	client *github.Client,
	owner, name string,
	ruleset orgsync.Ruleset,
) error {
	listed, err := client.ListRepositoryRulesets(ctx, owner, name)
	if err != nil {
		return err
	}

	for _, existing := range listed {
		if existing.Source.Inherited() {
			continue
		}

		if strings.EqualFold(existing.Name, ruleset.Name) {
			return fmt.Errorf("%w: %q was created since this plan", errSyncRulesetTaken, ruleset.Name)
		}
	}

	return client.CreateRepositoryRuleset(ctx, owner, name, asClientRuleset(ruleset))
}

// asClientRuleset carries a ruleset from the sync domain to the GitHub client,
// and asConfiguredRuleset carries one back.
//
// Mirror images, written out twice rather than shared, because neither package
// may import the other: the client must not know what sync is, and the sync
// domain must not know what go-github is. The same choice was made in chunk 4
// for the two readings of an installation's permissions, and for the same
// reason - the alternative is the client importing the domain it serves.
//
// What makes it safe rather than hopeful is TestARulesetSurvivesTheSeam, which
// walks a ruleset with every field set to a non-zero value out and back and
// refuses a fixture that has left one at its zero. A field added to one side
// and forgotten in the other fails that test rather than silently stopping
// being synchronized, which is how three of the tool this replaces came to be
// parsed, schema-validated and then dropped.
//
//nolint:dupl // Two directions of one seam; see the pin above.
func asClientRuleset(ruleset orgsync.Ruleset) github.RepositoryRuleset {
	converted := github.RepositoryRuleset{
		Name:        ruleset.Name,
		Target:      ruleset.Target,
		Enforcement: ruleset.Enforcement,
		Conditions: github.RulesetConditions{
			IncludeRefs: ruleset.Conditions.IncludeRefs,
			ExcludeRefs: ruleset.Conditions.ExcludeRefs,
		},
		Rules: asClientRules(ruleset.Rules),
	}

	for _, actor := range ruleset.BypassActors {
		converted.BypassActors = append(converted.BypassActors, github.RulesetBypassActor{
			ActorID:   actor.ActorID,
			ActorType: actor.ActorType,
			Mode:      actor.Mode,
		})
	}

	return converted
}

func asClientRules(rules orgsync.RulesetRules) github.RulesetRules {
	converted := github.RulesetRules{
		Creation:              rules.Creation,
		Deletion:              rules.Deletion,
		NonFastForward:        rules.NonFastForward,
		RequiredLinearHistory: rules.RequiredLinearHistory,
		RequiredSignatures:    rules.RequiredSignatures,
	}

	if update := rules.Update; update != nil {
		converted.Update = &github.RulesetUpdateRule{
			AllowsFetchAndMerge: update.AllowsFetchAndMerge,
		}
	}

	if pull := rules.PullRequest; pull != nil {
		converted.PullRequest = &github.RulesetPullRequestRule{
			RequiredApprovingReviewCount:   pull.RequiredApprovingReviewCount,
			DismissStaleReviewsOnPush:      pull.DismissStaleReviewsOnPush,
			RequireCodeOwnerReview:         pull.RequireCodeOwnerReview,
			RequireLastPushApproval:        pull.RequireLastPushApproval,
			RequiredReviewThreadResolution: pull.RequiredReviewThreadResolution,
			AllowedMergeMethods:            pull.AllowedMergeMethods,
		}
	}

	if checks := rules.RequiredStatusChecks; checks != nil {
		rule := &github.RulesetStatusChecksRule{
			Strict:               checks.Strict,
			DoNotEnforceOnCreate: checks.DoNotEnforceOnCreate,
		}

		for _, check := range checks.Checks {
			rule.Checks = append(rule.Checks, github.RulesetStatusCheck{
				Context:       check.Context,
				IntegrationID: check.IntegrationID,
			})
		}

		converted.RequiredStatusChecks = rule
	}

	if scanning := rules.CodeScanning; scanning != nil {
		rule := &github.RulesetCodeScanningRule{}

		for _, tool := range scanning.Tools {
			rule.Tools = append(rule.Tools, github.RulesetCodeScanningTool{
				Tool:                    tool.Tool,
				AlertsThreshold:         tool.AlertsThreshold,
				SecurityAlertsThreshold: tool.SecurityAlertsThreshold,
			})
		}

		converted.CodeScanning = rule
	}

	return converted
}

// asConfiguredRuleset is the other direction of the seam asClientRuleset
// describes.
//
//nolint:dupl // Two directions of one seam; see the pin on asClientRuleset.
func asConfiguredRuleset(ruleset github.RepositoryRuleset) orgsync.Ruleset {
	converted := orgsync.Ruleset{
		Name:        ruleset.Name,
		Target:      ruleset.Target,
		Enforcement: ruleset.Enforcement,
		Conditions: orgsync.RulesetConditions{
			IncludeRefs: ruleset.Conditions.IncludeRefs,
			ExcludeRefs: ruleset.Conditions.ExcludeRefs,
		},
		Rules: asConfiguredRules(ruleset.Rules),
	}

	for _, actor := range ruleset.BypassActors {
		converted.BypassActors = append(converted.BypassActors, orgsync.RulesetBypassActor{
			ActorID:   actor.ActorID,
			ActorType: actor.ActorType,
			Mode:      actor.Mode,
		})
	}

	return converted
}

func asConfiguredRules(rules github.RulesetRules) orgsync.RulesetRules {
	converted := orgsync.RulesetRules{
		Creation:              rules.Creation,
		Deletion:              rules.Deletion,
		NonFastForward:        rules.NonFastForward,
		RequiredLinearHistory: rules.RequiredLinearHistory,
		RequiredSignatures:    rules.RequiredSignatures,
	}

	if update := rules.Update; update != nil {
		converted.Update = &orgsync.RulesetUpdateRule{
			AllowsFetchAndMerge: update.AllowsFetchAndMerge,
		}
	}

	if pull := rules.PullRequest; pull != nil {
		converted.PullRequest = &orgsync.RulesetPullRequestRule{
			RequiredApprovingReviewCount:   pull.RequiredApprovingReviewCount,
			DismissStaleReviewsOnPush:      pull.DismissStaleReviewsOnPush,
			RequireCodeOwnerReview:         pull.RequireCodeOwnerReview,
			RequireLastPushApproval:        pull.RequireLastPushApproval,
			RequiredReviewThreadResolution: pull.RequiredReviewThreadResolution,
			AllowedMergeMethods:            pull.AllowedMergeMethods,
		}
	}

	if checks := rules.RequiredStatusChecks; checks != nil {
		rule := &orgsync.RulesetStatusChecksRule{
			Strict:               checks.Strict,
			DoNotEnforceOnCreate: checks.DoNotEnforceOnCreate,
		}

		for _, check := range checks.Checks {
			rule.Checks = append(rule.Checks, orgsync.RulesetStatusCheck{
				Context:       check.Context,
				IntegrationID: check.IntegrationID,
			})
		}

		converted.RequiredStatusChecks = rule
	}

	if scanning := rules.CodeScanning; scanning != nil {
		rule := &orgsync.RulesetCodeScanningRule{}

		for _, tool := range scanning.Tools {
			rule.Tools = append(rule.Tools, orgsync.RulesetCodeScanningTool{
				Tool:                    tool.Tool,
				AlertsThreshold:         tool.AlertsThreshold,
				SecurityAlertsThreshold: tool.SecurityAlertsThreshold,
			})
		}

		converted.CodeScanning = rule
	}

	return converted
}
