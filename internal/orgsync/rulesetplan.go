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

	// Unmanaged names what this ruleset enforces that a Ruleset cannot carry.
	//
	// A replacement removes it, because a replacement removes everything the
	// request does not repeat. Carried here so the plan can say which rules go,
	// rather than describing a change out of the half of the ruleset it could
	// read - somebody approving that would be approving a description with the
	// destruction left out of it.
	Unmanaged []string
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
//
// The second answer is the configured names this repository holds more than one
// of. Nothing here can say which one the configuration meant, so they produce
// no action - and a caller that read that as "nothing to do" would record the
// repository as matching and stop looking at it, which is the same silence in
// a different place.
func PlanRulesets(
	repositoryID string,
	config RulesetConfig,
	current []CurrentRuleset,
	exclude Excludes,
) (actions []Action, ambiguous []string) {
	have := indexRulesets(current)

	wanted := make(map[string]struct{}, len(config.Rulesets))

	for _, ruleset := range config.Rulesets {
		folded := strings.ToLower(ruleset.Name)
		wanted[folded] = struct{}{}

		if exclude.Matches(ruleset.Name) {
			continue
		}

		if len(have[folded].Own) > 1 {
			ambiguous = append(ambiguous, ruleset.Name)

			continue
		}

		if action, planned := planRuleset(repositoryID, ruleset, have[folded]); planned {
			actions = append(actions, action)
		}
	}

	actions = append(actions, removals(repositoryID, config, current, wanted, exclude)...)

	return actions, ambiguous
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

// planRuleset answers what one configured ruleset would take, for a repository
// holding at most one of that name. More than one is answered by the caller,
// because there is no action that would say it.
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

	default:
		return planRulesetUpdate(repositoryID, want, have.Own[0], note)
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

	// A rule this version cannot carry counts as a difference, always. It is
	// enforced now and a replacement removes it, so a repository holding one
	// has never matched however well the rest lines up - and reporting it
	// settled would leave the removal to happen the next time anything else
	// changed, in a plan that said nothing about it.
	if len(have.Unmanaged) == 0 && sameRuleset(want, *have.Defined) {
		return Action{}, false
	}

	return Action{
		RepositoryID: repositoryID,
		Kind:         KindRulesets,
		Operation:    OperationUpdate,
		Subject:      want.Name,
		Before:       describeRuleset(*have.Defined),
		After:        describeRuleset(want) + dropped(have.Unmanaged) + note,
		Payload:      encodeRuleset(withID(want, have.ID)),
		State:        ActionPending,
	}, true
}

// dropped names what a replacement takes away that this version never modelled.
//
// On After rather than Before, because After is what will be true and this is a
// consequence of the change rather than a description of what is there now. The
// answer for nearly every ruleset is nothing at all.
func dropped(unmanaged []string) string {
	if len(unmanaged) == 0 {
		return ""
	}

	return "; this drops " + strings.Join(unmanaged, ", ") +
		", which this version cannot express"
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

	ruleset.BypassActors = sortedBy(
		mapped(ruleset.BypassActors, canonicalActor), actorKey)
	ruleset.Rules = canonicalRules(ruleset.Rules)

	return ruleset
}

// canonicalActor is one bypass actor in the one spelling used for comparison.
//
// An organization admin is a role rather than somebody, so GitHub answers with
// no id for it however it was written - configuration has to carry one, because
// the create is refused without it, and reading the ruleset back always gives
// null. Comparing that id is how a repository already enforcing exactly the
// configured ruleset is rewritten on every tick for ever: the write succeeds,
// the read says null again, and the next sweep proposes the same change.
//
// Only this type. The other four name somebody GitHub can hand back - an app,
// a team, a role, a deploy key - and dropping their ids would make two actors
// that differ compare the same.
func canonicalActor(actor RulesetBypassActor) RulesetBypassActor {
	if actor.ActorType == bypassActorOrganizationAdmin {
		actor.ActorID = 0
	}

	return actor
}

func canonicalRules(rules RulesetRules) RulesetRules {
	if pull := rules.PullRequest; pull != nil {
		canonical := *pull
		canonical.AllowedMergeMethods = sorted(canonical.AllowedMergeMethods)
		rules.PullRequest = &canonical
	}

	if checks := rules.RequiredStatusChecks; checks != nil {
		canonical := *checks
		canonical.Checks = sortedBy(checks.Checks,
			func(check RulesetStatusCheck) string { return check.Context })
		rules.RequiredStatusChecks = &canonical
	}

	if scanning := rules.CodeScanning; scanning != nil {
		canonical := *scanning
		canonical.Tools = sortedBy(scanning.Tools,
			func(tool RulesetCodeScanningTool) string { return tool.Tool })
		rules.CodeScanning = &canonical
	}

	return rules
}

// sortedBy is one list in the one spelling used for comparison: a copy, in the
// order of a key, with empty spelled as nil rather than as a list of nothing.
//
// The copy matters. Everything reaching this is either the stored configuration
// or what GitHub answered, and sorting either in place would leave the plan
// carrying a payload in an order nobody wrote.
func sortedBy[T any](values []T, key func(T) string) []T {
	if len(values) == 0 {
		return nil
	}

	canonical := slices.Clone(values)
	sort.Slice(canonical, func(i, j int) bool {
		return key(canonical[i]) < key(canonical[j])
	})

	return canonical
}

func sorted(values []string) []string {
	return sortedBy(values, func(value string) string { return value })
}

// mapped is every value through one function, empty spelled as nil to match
// sortedBy. A copy for the same reason: what reaches here is the stored
// configuration or GitHub's answer, and neither is this function's to rewrite.
func mapped[T any](values []T, through func(T) T) []T {
	if len(values) == 0 {
		return nil
	}

	canonical := make([]T, 0, len(values))
	for _, value := range values {
		canonical = append(canonical, through(value))
	}

	return canonical
}

// actorKey orders bypass actors by everything about them, because nothing about
// them is unique on its own: one actor may appear twice under two modes, and
// two actor types share an id space.
func actorKey(actor RulesetBypassActor) string {
	return actor.ActorType + "\x00" + strconv.FormatInt(actor.ActorID, 10) +
		"\x00" + actor.Mode
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

// describeRules joins what a ruleset enforces into one line.
func describeRules(rules RulesetRules) string {
	return strings.Join(rules.Named(), ", ")
}

// Named is what a ruleset enforces, one phrase per rule.
//
// The list rather than the joined line, because a reader that draws the
// rules one at a time and one that writes them into a sentence are asking
// the same question, and a second walk over this struct in the panel would
// be a second place to update when a rule is added.
func (r RulesetRules) Named() []string {
	var described []string

	for _, rule := range []struct {
		on   bool
		name string
	}{
		{r.Creation, "no creation"},
		{r.Deletion, "no deletion"},
		{r.NonFastForward, "no force pushes"},
		{r.RequiredLinearHistory, "linear history"},
		{r.RequiredSignatures, "signed commits"},
	} {
		if rule.on {
			described = append(described, rule.name)
		}
	}

	if r.Update != nil {
		if r.Update.AllowsFetchAndMerge {
			described = append(described, "updates only by fetch and merge")
		} else {
			described = append(described, "no updates")
		}
	}

	if pull := r.PullRequest; pull != nil {
		described = append(described, describePullRequest(*pull))
	}

	if checks := r.RequiredStatusChecks; checks != nil {
		described = append(described, describeChecks(*checks))
	}

	if scanning := r.CodeScanning; scanning != nil {
		tools := make([]string, 0, len(scanning.Tools))
		for _, tool := range scanning.Tools {
			tools = append(tools, fmt.Sprintf("%s at %s, security %s",
				tool.Tool, tool.AlertsThreshold, tool.SecurityAlertsThreshold))
		}

		described = append(described, within("code scanning", tools))
	}

	return described
}

// within puts a rule's own list inside brackets under its name.
//
// Everything here is a comma list, and a rule that carries one of its own -
// the reviews a pull request needs, the checks that must pass, the tools -
// would otherwise run its items into its neighbours. `linear history, 1
// approving review, from code owners, no force pushes` reads as four rules and
// is two.
func within(rule string, parts []string) string {
	if len(parts) == 0 {
		return rule
	}

	return rule + " (" + strings.Join(parts, ", ") + ")"
}

func describePullRequest(rule RulesetPullRequestRule) string {
	described := []string{reviews(rule.RequiredApprovingReviewCount)}

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

	return within("pull requests", described)
}

// reviews counts approvals in words a person would use. "1 approving
// review(s)" is the shape of a rendering nobody read back.
func reviews(count int) string {
	switch count {
	case 0:
		return "no approving review"
	case 1:
		return "1 approving review"
	default:
		return fmt.Sprintf("%d approving reviews", count)
	}
}

func describeChecks(rule RulesetStatusChecksRule) string {
	described := make([]string, 0, len(rule.Checks)+2)
	for _, check := range rule.Checks {
		described = append(described, check.Context)
	}

	if rule.Strict {
		described = append(described, "branch up to date")
	}
	if rule.DoNotEnforceOnCreate {
		described = append(described, "not on a new branch")
	}

	return within("checks", described)
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
