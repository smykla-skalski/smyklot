package orgsync

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// CurrentRuleset is a ruleset as GitHub currently has it.
//
// Two levels of detail on purpose. GitHub's listing carries identity and no
// rules, and reading the whole of one costs a request each, so the caller reads
// whole only the rulesets configuration names and leaves the rest as they came.
type CurrentRuleset struct {
	ID   int64
	Name string

	// Inherited is a ruleset defined on the organization or the enterprise.
	//
	// Not this repository's to change and not its to delete, and the reason
	// they are read at all: a repository-level ruleset of the same name does
	// not replace an inherited one, it stacks on it, and GitHub then enforces
	// the union. That is invisible until somebody wonders why their merge is
	// refused by a rule they can see nowhere in this configuration.
	Inherited bool

	// Defined is what the ruleset enforces, or nil where only the listing was
	// read.
	Defined *Ruleset
}

// PlanRulesets answers what would have to change for a repository's rulesets to
// match its configuration.
//
// Pure, like every planner here: it reaches nothing and returns actions rather
// than performing them.
//
// Keyed by name, folded, because a name is the only handle there is. GitHub
// gives a ruleset an id it mints itself and permits two rulesets with the same
// name, so the tool this replaces - which read one page of thirty, missed the
// rest, and created what it could not see - ended up updating whichever
// duplicate came back first.
func PlanRulesets(
	repositoryID string,
	config RulesetConfig,
	current []CurrentRuleset,
	exclude Excludes,
) []Action {
	have := indexRulesets(current)

	var (
		actions []Action
		wanted  = make(map[string]struct{}, len(config.Rulesets))
	)

	for _, ruleset := range config.Rulesets {
		folded := strings.ToLower(ruleset.Name)
		wanted[folded] = struct{}{}

		if exclude.Matches(ruleset.Name) {
			continue
		}

		if action, planned := planRuleset(repositoryID, ruleset, have[folded]); planned {
			actions = append(actions, action)
		}
	}

	return append(actions, removals(repositoryID, config, current, wanted, exclude)...)
}

// rulesetsNamed is every ruleset a repository has under one folded name.
type rulesetsNamed struct {
	// Own is the ones defined on the repository itself, in listing order.
	Own []CurrentRuleset

	// Inherited reports at least one of this name coming from above.
	Inherited bool
}

func indexRulesets(current []CurrentRuleset) map[string]rulesetsNamed {
	have := make(map[string]rulesetsNamed, len(current))

	for _, ruleset := range current {
		folded := strings.ToLower(ruleset.Name)
		named := have[folded]

		if ruleset.Inherited {
			named.Inherited = true
		} else {
			named.Own = append(named.Own, ruleset)
		}

		have[folded] = named
	}

	return have
}

// planRuleset answers what one configured ruleset would take.
//
// Three of the four answers are actions. The fourth is nothing at all, and it
// covers two cases worth keeping apart in the reading: a repository that
// already matches, and one this cannot safely act on.
func planRuleset(
	repositoryID string,
	want Ruleset,
	have rulesetsNamed,
) (Action, bool) {
	note := ""
	if have.Inherited {
		// Said on the action rather than withheld from it. The two rulesets do
		// not replace each other, they both enforce, so refusing to write this
		// one would be deciding on somebody's behalf that the organization's is
		// the one they meant. Naming it puts the choice in front of whoever
		// approves the plan, at the one moment the stack is created.
		note = "; an organization ruleset of this name also applies here"
	}

	switch len(have.Own) {
	case 0:
		return Action{
			RepositoryID: repositoryID,
			Kind:         KindRulesets,
			Operation:    OperationCreate,
			Subject:      want.Name,
			After:        describeRuleset(want) + note,
			Payload:      encodeRuleset(withID(want, 0)),
			State:        ActionPending,
		}, true

	case 1:
		return planRulesetUpdate(repositoryID, want, have.Own[0], note)

	default:
		// More than one of this name, on this repository. Nothing here can say
		// which one the configuration meant, and writing to either leaves the
		// other enforcing whatever it enforces - which is the state the tool
		// this replaces created, by making duplicates and then updating an
		// arbitrary one of them for ever.
		//
		// So: nothing. The repository does not settle for this kind, which
		// costs it a re-read each sweep and means the next one picks it up the
		// moment somebody removes the copy.
		return Action{}, false
	}
}

func planRulesetUpdate(
	repositoryID string,
	want Ruleset,
	have CurrentRuleset,
	note string,
) (Action, bool) {
	if have.Defined == nil {
		// The listing said this exists and nothing read what it enforces.
		// Writing now would be the blind full replace this chunk exists to
		// remove: a request that drops every rule, condition and bypass actor
		// the repository has, computed from an answer nobody received.
		return Action{}, false
	}

	if sameRuleset(want, *have.Defined) {
		return Action{}, false
	}

	return Action{
		RepositoryID: repositoryID,
		Kind:         KindRulesets,
		Operation:    OperationUpdate,
		Subject:      want.Name,
		Before:       describeRuleset(*have.Defined),
		After:        describeRuleset(want) + note,
		Payload:      encodeRuleset(withID(want, have.ID)),
		State:        ActionPending,
	}, true
}

// removals proposes deleting the rulesets configuration no longer names.
//
// The tool this replaces had no delete path at all, so a ruleset dropped from
// configuration went on enforcing for ever and a rename left two of them. Off
// unless removal is switched on, and even then it is a proposal: the plan
// carries what would go, and somebody approves it.
//
// Only ever the repository's own. An inherited ruleset cannot be deleted
// through the repository's endpoint, and proposing it would put work in a plan
// that could only fail.
func removals(
	repositoryID string,
	config RulesetConfig,
	current []CurrentRuleset,
	wanted map[string]struct{},
	exclude Excludes,
) []Action {
	if !config.AllowRemoval {
		return nil
	}

	surplus := make([]CurrentRuleset, 0, len(current))

	for _, ruleset := range current {
		if ruleset.Inherited {
			continue
		}
		if _, keep := wanted[strings.ToLower(ruleset.Name)]; keep {
			continue
		}
		if exclude.Matches(ruleset.Name) {
			continue
		}

		surplus = append(surplus, ruleset)
	}

	// Sorted, because the answer must not depend on the order GitHub happened
	// to list them in. Two plans of the same state have to be the same plan, or
	// the digest comparison means nothing. By id after name, because two
	// rulesets may share one.
	sort.Slice(surplus, func(i, j int) bool {
		if surplus[i].Name != surplus[j].Name {
			return surplus[i].Name < surplus[j].Name
		}

		return surplus[i].ID < surplus[j].ID
	})

	actions := make([]Action, 0, len(surplus))

	for _, ruleset := range surplus {
		actions = append(actions, Action{
			RepositoryID: repositoryID,
			Kind:         KindRulesets,
			Operation:    OperationDelete,
			Subject:      ruleset.Name,
			Before:       describeRemoved(ruleset),
			Payload:      encodeRuleset(withID(Ruleset{Name: ruleset.Name}, ruleset.ID)),
			State:        ActionPending,
		})
	}

	return actions
}

// ResolvedRuleset is a ruleset with the one thing configuration cannot know
// answered: which ruleset on GitHub it is.
//
// The id is decided when the plan is computed and carried to the executor,
// rather than looked up again when the work runs. Looking it up again is how a
// ruleset created between approval and apply comes to be replaced by a plan
// nobody wrote about it.
type ResolvedRuleset struct {
	Ruleset

	// ID is the ruleset to write to, zero for one that does not exist yet.
	ID int64 `json:"id,omitempty"`
}

func withID(ruleset Ruleset, id int64) ResolvedRuleset {
	return ResolvedRuleset{Ruleset: ruleset, ID: id}
}

// DecodeRulesetAction reads what an action says to apply.
func DecodeRulesetAction(payload []byte) (ResolvedRuleset, error) {
	var ruleset ResolvedRuleset
	if err := json.Unmarshal(payload, &ruleset); err != nil {
		return ResolvedRuleset{}, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}

	return ruleset, nil
}

func encodeRuleset(ruleset ResolvedRuleset) []byte {
	// Strings, booleans, integers and slices of them. There is no value here
	// that can fail to encode, and returning an error would make every caller
	// handle one that cannot happen.
	payload, _ := json.Marshal(ruleset)

	return payload
}

// sameRuleset reports two rulesets that would enforce the same thing.
//
// Through a canonical form rather than field by field, because the two sides
// arrive spelled differently and none of the differences mean anything. GitHub
// answers with empty lists where configuration left them out and lists in
// whatever order it stored them; comparing those literally is how a repository
// that already matches gets rewritten on every tick for ever.
func sameRuleset(want, have Ruleset) bool {
	return reflect.DeepEqual(canonicalRuleset(want), canonicalRuleset(have))
}

// canonicalRuleset is one ruleset in the one spelling used for comparison.
//
// Every list is sorted and every empty list is dropped. Order carries no
// meaning in any of them - a set of refs, of merge methods, of checks, of
// actors - and GitHub does not promise to give back the order it was given.
func canonicalRuleset(ruleset Ruleset) Ruleset {
	ruleset.Conditions = RulesetConditions{
		IncludeRefs: sorted(ruleset.Conditions.IncludeRefs),
		ExcludeRefs: sorted(ruleset.Conditions.ExcludeRefs),
	}

	ruleset.BypassActors = canonicalActors(ruleset.BypassActors)
	ruleset.Rules = canonicalRules(ruleset.Rules)

	return ruleset
}

func canonicalActors(actors []RulesetBypassActor) []RulesetBypassActor {
	if len(actors) == 0 {
		return nil
	}

	canonical := slices.Clone(actors)
	sort.Slice(canonical, func(i, j int) bool {
		return actorKey(canonical[i]) < actorKey(canonical[j])
	})

	return canonical
}

func actorKey(actor RulesetBypassActor) string {
	return actor.ActorType + "\x00" + strconv.FormatInt(actor.ActorID, 10) +
		"\x00" + actor.Mode
}

func canonicalRules(rules RulesetRules) RulesetRules {
	if pull := rules.PullRequest; pull != nil {
		canonical := *pull
		canonical.AllowedMergeMethods = sorted(canonical.AllowedMergeMethods)
		rules.PullRequest = &canonical
	}

	if checks := rules.RequiredStatusChecks; checks != nil {
		canonical := *checks
		canonical.Checks = slices.Clone(checks.Checks)
		sort.Slice(canonical.Checks, func(i, j int) bool {
			return canonical.Checks[i].Context < canonical.Checks[j].Context
		})
		if len(canonical.Checks) == 0 {
			canonical.Checks = nil
		}
		rules.RequiredStatusChecks = &canonical
	}

	if scanning := rules.CodeScanning; scanning != nil {
		canonical := *scanning
		canonical.Tools = slices.Clone(scanning.Tools)
		sort.Slice(canonical.Tools, func(i, j int) bool {
			return canonical.Tools[i].Tool < canonical.Tools[j].Tool
		})
		if len(canonical.Tools) == 0 {
			canonical.Tools = nil
		}
		rules.CodeScanning = &canonical
	}

	return rules
}

func sorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	canonical := slices.Clone(values)
	sort.Strings(canonical)

	return canonical
}

// describeRuleset renders a ruleset for a person reading the plan. It is
// display, never a value anything branches on.
//
// Long, because a ruleset is. What is being approved here is a replacement of
// everything the repository has under this name, so the reader has to be able
// to see everything that will be there afterwards - and, in Before, everything
// that will not.
func describeRuleset(ruleset Ruleset) string {
	parts := []string{ruleset.Target + ", " + ruleset.Enforcement}

	if refs := ruleset.Conditions.IncludeRefs; len(refs) > 0 {
		parts = append(parts, "on "+strings.Join(refs, ", "))
	}
	if refs := ruleset.Conditions.ExcludeRefs; len(refs) > 0 {
		parts = append(parts, "except "+strings.Join(refs, ", "))
	}

	if rules := describeRules(ruleset.Rules); rules != "" {
		parts = append(parts, rules)
	} else {
		// Worth saying out loud. A ruleset that enforces nothing looks from its
		// name and its refs exactly like one that does.
		parts = append(parts, "enforcing nothing")
	}

	if actors := describeActors(ruleset.BypassActors); actors != "" {
		parts = append(parts, actors)
	}

	return strings.Join(parts, "; ")
}

// describeRemoved renders a ruleset that is about to go.
//
// Only its name is certain: a surplus ruleset is one nothing read whole, so
// there is nothing else honest to print. Saying so beats printing an empty
// ruleset, which would read as one that enforced nothing.
func describeRemoved(ruleset CurrentRuleset) string {
	if ruleset.Defined == nil {
		return fmt.Sprintf("%s, whatever it enforces", ruleset.Name)
	}

	return describeRuleset(*ruleset.Defined)
}

func describeRules(rules RulesetRules) string {
	var described []string

	for _, rule := range []struct {
		on   bool
		name string
	}{
		{rules.Creation, "no creation"},
		{rules.Deletion, "no deletion"},
		{rules.NonFastForward, "no force pushes"},
		{rules.RequiredLinearHistory, "linear history"},
		{rules.RequiredSignatures, "signed commits"},
	} {
		if rule.on {
			described = append(described, rule.name)
		}
	}

	if rules.Update != nil {
		if rules.Update.AllowsFetchAndMerge {
			described = append(described, "updates only by fetch and merge")
		} else {
			described = append(described, "no updates")
		}
	}

	if pull := rules.PullRequest; pull != nil {
		described = append(described, describePullRequest(*pull))
	}

	if checks := rules.RequiredStatusChecks; checks != nil {
		described = append(described, describeChecks(*checks))
	}

	if scanning := rules.CodeScanning; scanning != nil {
		for _, tool := range scanning.Tools {
			described = append(described, fmt.Sprintf("%s at %s, security %s",
				tool.Tool, tool.AlertsThreshold, tool.SecurityAlertsThreshold))
		}
	}

	return strings.Join(described, ", ")
}

func describePullRequest(rule RulesetPullRequestRule) string {
	described := []string{fmt.Sprintf("%d approving review(s)",
		rule.RequiredApprovingReviewCount)}

	for _, flag := range []struct {
		on   bool
		name string
	}{
		{rule.RequireCodeOwnerReview, "from code owners"},
		{rule.DismissStaleReviewsOnPush, "dismissed on push"},
		{rule.RequireLastPushApproval, "covering the last push"},
		{rule.RequiredReviewThreadResolution, "threads resolved"},
	} {
		if flag.on {
			described = append(described, flag.name)
		}
	}

	if len(rule.AllowedMergeMethods) > 0 {
		described = append(described,
			"merged by "+strings.Join(rule.AllowedMergeMethods, " or "))
	}

	return strings.Join(described, ", ")
}

func describeChecks(rule RulesetStatusChecksRule) string {
	contexts := make([]string, 0, len(rule.Checks))
	for _, check := range rule.Checks {
		contexts = append(contexts, check.Context)
	}

	described := "checks " + strings.Join(contexts, ", ")
	if rule.Strict {
		described += ", up to date"
	}
	if rule.DoNotEnforceOnCreate {
		described += ", not on creation"
	}

	return described
}

func describeActors(actors []RulesetBypassActor) string {
	if len(actors) == 0 {
		return ""
	}

	described := make([]string, 0, len(actors))
	for _, actor := range actors {
		described = append(described, fmt.Sprintf("%s %d %s",
			actor.ActorType, actor.ActorID, actor.Mode))
	}

	return "bypassed by " + strings.Join(described, ", ")
}
