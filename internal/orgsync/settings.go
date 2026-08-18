package orgsync

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// SettingsConfig is what an installation expects its repositories to be set to.
//
// Every field is a pointer, without exception. A repository setting has three
// states and not two: configured on, configured off, and not configured at all.
// The tool this replaces used plain bools for half of them and omitted the rest,
// so "nobody said" and "somebody said no" were the same value - and against an
// endpoint that replaces what it is sent, that silently turned features off that
// nobody had asked about.
type SettingsConfig struct {
	// How a pull request may be merged.
	AllowMergeCommit *bool `json:"allow_merge_commit,omitempty"`
	AllowSquashMerge *bool `json:"allow_squash_merge,omitempty"`
	AllowRebaseMerge *bool `json:"allow_rebase_merge,omitempty"`
	AllowAutoMerge   *bool `json:"allow_auto_merge,omitempty"`

	// What happens around a merge.
	DeleteBranchOnMerge *bool `json:"delete_branch_on_merge,omitempty"`

	// AllowUpdateBranch is one of the three the tool this replaces parsed,
	// validated against its own schema, and then never sent.
	AllowUpdateBranch *bool `json:"allow_update_branch,omitempty"`

	// How a squash or merge commit is worded. The other two that were parsed
	// and dropped.
	SquashMergeCommitTitle   *string `json:"squash_merge_commit_title,omitempty"`
	SquashMergeCommitMessage *string `json:"squash_merge_commit_message,omitempty"`
	MergeCommitTitle         *string `json:"merge_commit_title,omitempty"`
	MergeCommitMessage       *string `json:"merge_commit_message,omitempty"`

	// Which features a repository offers.
	HasIssues      *bool `json:"has_issues,omitempty"`
	HasProjects    *bool `json:"has_projects,omitempty"`
	HasWiki        *bool `json:"has_wiki,omitempty"`
	HasDiscussions *bool `json:"has_discussions,omitempty"`

	// Which security features it has switched on. Configured as booleans like
	// everything else here, though GitHub takes them nested under
	// security_and_analysis with a status string: a person configuring an
	// organization should not have to know which half of the endpoint a
	// setting lives in.
	AdvancedSecurity             *bool `json:"advanced_security,omitempty"`
	SecretScanning               *bool `json:"secret_scanning,omitempty"`
	SecretScanningPushProtection *bool `json:"secret_scanning_push_protection,omitempty"`

	// DependabotSecurityUpdates is configured here with the security features
	// it belongs beside, and is not one of settingsFields: GitHub reports it
	// inside security_and_analysis and refuses to change it there, so it is
	// planned as an action of its own. See PlanSettings.
	DependabotSecurityUpdates *bool `json:"dependabot_security_updates,omitempty"`
}

// The values GitHub accepts for how a commit is worded. Checked here because
// anything else is a 422 that abandons the whole settings change, and the tool
// this replaces passed these through unvalidated.
//
// Spelled as constants because the same value is legal for more than one field
// and means the same thing in each: a second spelling of PR_TITLE is a place
// for one of them to be typed wrong.
const (
	sourcePRTitle       = "PR_TITLE"
	sourcePRBody        = "PR_BODY"
	sourceBlank         = "BLANK"
	sourceCommitOrTitle = "COMMIT_OR_PR_TITLE"
	sourceCommits       = "COMMIT_MESSAGES"
	sourceMergeMessage  = "MERGE_MESSAGE"
)

var (
	squashTitles   = []string{sourcePRTitle, sourceCommitOrTitle}
	squashMessages = []string{sourcePRBody, sourceCommits, sourceBlank}
	mergeTitles    = []string{sourcePRTitle, sourceMergeMessage}
	mergeMessages  = []string{sourcePRBody, sourcePRTitle, sourceBlank}
)

// Validate reports configuration GitHub would refuse.
func (c SettingsConfig) Validate() error {
	for _, field := range []struct {
		name    string
		value   *string
		allowed []string
	}{
		{"squash_merge_commit_title", c.SquashMergeCommitTitle, squashTitles},
		{"squash_merge_commit_message", c.SquashMergeCommitMessage, squashMessages},
		{"merge_commit_title", c.MergeCommitTitle, mergeTitles},
		{"merge_commit_message", c.MergeCommitMessage, mergeMessages},
	} {
		if field.value == nil {
			continue
		}
		if !slices.Contains(field.allowed, *field.value) {
			return invalid("%s must be one of %s, not %q",
				field.name, strings.Join(field.allowed, ", "), *field.value)
		}
	}

	fields := settingsFields()
	byName := fieldsByName(fields)

	// A repository has to allow some way of merging. GitHub refuses the last
	// one being turned off, and it refuses it as a 422 on the whole request -
	// so a configuration that turns all three off breaks every other setting
	// in the same change rather than only itself.
	//
	// All three, because two off is a legal thing to ask for: a repository
	// whose third one is on takes it. What that leaves - two off and one
	// unconfigured, against a repository that already has the third off - is
	// caught by DiffSettings, which is the only place that knows.
	if noMergeMethod(fields, func(field settingsField) (bool, bool) {
		value, _, configured := field.want(c)
		enabled, _ := value.(bool)

		return enabled, configured
	}) {
		return invalid("a repository must allow at least one way to merge")
	}

	// A setting no repository could ever accept. GitHub judges a commit wording
	// against the merge strategy beside it, and a security feature against what
	// it is built on, and refuses the pair as a 422 on the whole request - so
	// DiffSettings withholds one from a repository that has the other off,
	// which is what keeps a squash-only repository from losing its whole
	// settings change over a merge commit title. A configuration that turns the
	// other off itself is asking for something impossible everywhere, and the
	// place to say so is beside the field somebody typed rather than in every
	// plan, silently.

	for _, field := range fields {
		if !field.asking(c) {
			continue
		}

		if under := turnedOff(byName, c, field); under != "" {
			return invalid("%s needs %s, which this configuration turns off",
				field.name, under)
		}
	}

	return nil
}

// turnedOff names the first setting under this one that the configuration
// switches off, however far under it that is.
//
// A chain is no more reachable for its length: push protection needs secret
// scanning, which needs advanced security, and a configuration turning that
// last one off puts the first out of reach as surely as turning off the one
// directly beneath it. What is named is what was turned off rather than the
// link in between, because that is the line somebody has to change.
func turnedOff(
	byName map[string]settingsField, config SettingsConfig, field settingsField,
) string {
	// One step per field in the table. A chain reaches each of them at most
	// once, and a table that ever described a circle would hang here rather
	// than answer.
	for range byName {
		under, known := byName[field.requires]
		if !known {
			return ""
		}

		if value, _, configured := under.want(config); configured && value == false {
			return under.name
		}

		field = under
	}

	return ""
}

// CurrentSettings is a repository's settings as GitHub reports them.
//
// Values rather than pointers, and deliberately: GitHub answers with every one
// of these, so absent has no meaning here. The asymmetry with SettingsConfig is
// the whole point - configuration may decline to say, an observation cannot.
type CurrentSettings struct {
	AllowMergeCommit    bool
	AllowSquashMerge    bool
	AllowRebaseMerge    bool
	AllowAutoMerge      bool
	DeleteBranchOnMerge bool
	AllowUpdateBranch   bool

	SquashMergeCommitTitle   string
	SquashMergeCommitMessage string
	MergeCommitTitle         string
	MergeCommitMessage       string

	HasIssues      bool
	HasProjects    bool
	HasWiki        bool
	HasDiscussions bool

	// The security features, which are the one part of a repository with three
	// states rather than two.
	AdvancedSecurity             FeatureState
	SecretScanning               FeatureState
	SecretScanningPushProtection FeatureState

	// DependabotSecurityUpdates has the same three states as the others and is
	// read from the same object. Only the writing is different.
	DependabotSecurityUpdates FeatureState
}

// FeatureState is what a repository reports about a security feature.
//
// Three values, because GitHub omits a feature the repository cannot have
// rather than reporting it off - and the difference decides whether sync should
// try. Reading absence as off is how the tool this replaces came to send
// "enable advanced security" to a repository that has no such thing on every
// single run, and to lose the whole settings change to the 422 that came back.
type FeatureState string

const (
	FeatureOn  FeatureState = "on"
	FeatureOff FeatureState = "off"

	// FeatureUnavailable is a feature this repository does not offer. Nothing
	// can be done about it here: it is a plan or a licence, bought elsewhere.
	FeatureUnavailable FeatureState = "unavailable"
)

// Reported answers whether GitHub said anything about this feature.
//
// Only the two states it names count. The zero value is neither, and reading it
// as "has the feature, switched off" would be reading a repository nobody
// described as one that refused something - which is how an unset field in a
// caller becomes a change GitHub answers with a 422.
func (f FeatureState) Reported() bool { return f == FeatureOn || f == FeatureOff }

// settingsField is one setting, named once for the three things that have to
// agree about it: what to compare, what to send, and what to show a person.
//
// want answers all three at once - the value for the request, its rendering for
// a reader, and whether it was configured at all - because they are one fact.
// Deriving them separately gives each a chance to disagree about whether a
// setting was configured, and the two guards that produced would mask each
// other: break either and every spec still passes.
type settingsField struct {
	name string

	// requires names the merge strategy this setting is meaningless without,
	// and is empty for one that stands on its own. GitHub refuses a squash
	// commit title on a repository that does not allow squash merges, and it
	// refuses it as a 422 on the whole request - so a wording the repository
	// cannot use would take down every other setting sent beside it.
	requires string

	// merges marks one of the ways a repository may be merged. GitHub judges
	// those as a group rather than one at a time: at least one has to be left
	// on, and the last one being turned off is a 422 on the whole request.
	merges bool

	want func(SettingsConfig) (value any, display string, configured bool)
	have func(CurrentSettings) string

	// on is what this setting would be once the change lands, for the settings
	// something else depends on. Nil where nothing can: only a boolean is ever
	// the subject of a requires.
	on func(SettingsConfig, CurrentSettings) bool

	// now is the same setting as the repository has it, which is what it keeps
	// when the change to it is withheld. Nil beside a nil on, for the same
	// reason.
	now func(CurrentSettings) bool

	// available reports the repository having this setting at all, for the ones
	// it may not. Nil where every repository has it, which is all of them
	// except the security features.
	available func(CurrentSettings) bool

	// put writes the value where GitHub expects it in the request body. Most
	// settings are a key of their own; the security features are nested under
	// security_and_analysis with a status string rather than a boolean, and the
	// body is the plan's contract, so the shape is decided here rather than
	// rebuilt by whoever sends it.
	put func(body map[string]any, value any)
}

// settingsFields is every setting the settings endpoint itself takes, in the
// order a person reads them.
//
// Not quite every setting: DependabotSecurityUpdates is configured beside these
// and is deliberately absent, because GitHub changes it somewhere else. That
// makes this table the answer to "what goes in the one request", which is what
// every reader of it wants - the merge-strategy rule, the dependency chains and
// what a change switches off with it are all rules about that request. See
// planDependabot for the setting that is not in it.
//
// One table rather than a comparison, a request body and a description written
// separately. Those three drifting apart is how the tool this replaces came to
// validate three fields it never sent.
func settingsFields() []settingsField {
	return []settingsField{
		merging(boolField("allow_merge_commit",
			func(c SettingsConfig) *bool { return c.AllowMergeCommit },
			func(s CurrentSettings) bool { return s.AllowMergeCommit })),
		merging(boolField("allow_squash_merge",
			func(c SettingsConfig) *bool { return c.AllowSquashMerge },
			func(s CurrentSettings) bool { return s.AllowSquashMerge })),
		merging(boolField("allow_rebase_merge",
			func(c SettingsConfig) *bool { return c.AllowRebaseMerge },
			func(s CurrentSettings) bool { return s.AllowRebaseMerge })),
		boolField("allow_auto_merge",
			func(c SettingsConfig) *bool { return c.AllowAutoMerge },
			func(s CurrentSettings) bool { return s.AllowAutoMerge }),
		boolField("delete_branch_on_merge",
			func(c SettingsConfig) *bool { return c.DeleteBranchOnMerge },
			func(s CurrentSettings) bool { return s.DeleteBranchOnMerge }),
		boolField("allow_update_branch",
			func(c SettingsConfig) *bool { return c.AllowUpdateBranch },
			func(s CurrentSettings) bool { return s.AllowUpdateBranch }),
		textField("squash_merge_commit_title", "allow_squash_merge",
			func(c SettingsConfig) *string { return c.SquashMergeCommitTitle },
			func(s CurrentSettings) string { return s.SquashMergeCommitTitle }),
		textField("squash_merge_commit_message", "allow_squash_merge",
			func(c SettingsConfig) *string { return c.SquashMergeCommitMessage },
			func(s CurrentSettings) string { return s.SquashMergeCommitMessage }),
		textField("merge_commit_title", "allow_merge_commit",
			func(c SettingsConfig) *string { return c.MergeCommitTitle },
			func(s CurrentSettings) string { return s.MergeCommitTitle }),
		textField("merge_commit_message", "allow_merge_commit",
			func(c SettingsConfig) *string { return c.MergeCommitMessage },
			func(s CurrentSettings) string { return s.MergeCommitMessage }),
		boolField("has_issues",
			func(c SettingsConfig) *bool { return c.HasIssues },
			func(s CurrentSettings) bool { return s.HasIssues }),
		boolField("has_projects",
			func(c SettingsConfig) *bool { return c.HasProjects },
			func(s CurrentSettings) bool { return s.HasProjects }),
		boolField("has_wiki",
			func(c SettingsConfig) *bool { return c.HasWiki },
			func(s CurrentSettings) bool { return s.HasWiki }),
		boolField("has_discussions",
			func(c SettingsConfig) *bool { return c.HasDiscussions },
			func(s CurrentSettings) bool { return s.HasDiscussions }),
		securityField("advanced_security", "",
			func(c SettingsConfig) *bool { return c.AdvancedSecurity },
			func(s CurrentSettings) FeatureState { return s.AdvancedSecurity }),
		// GitHub refuses secret scanning on a repository whose advanced
		// security is off - "secret scanning can only be enabled on repos where
		// Advanced Security is enabled" - and refuses it as a 422 on the whole
		// request. A repository that has no such thing to turn on is a
		// different case and is not held to this: see how a dependency that is
		// unavailable is read in DiffSettings.
		securityField("secret_scanning", "advanced_security",
			func(c SettingsConfig) *bool { return c.SecretScanning },
			func(s CurrentSettings) FeatureState { return s.SecretScanning }),
		// GitHub refuses push protection on a repository whose secret scanning
		// is off, which is the same rule as a commit wording needing its merge
		// strategy and is enforced by the same field.
		securityField("secret_scanning_push_protection", "secret_scanning",
			func(c SettingsConfig) *bool { return c.SecretScanningPushProtection },
			func(s CurrentSettings) FeatureState { return s.SecretScanningPushProtection }),
	}
}

// securitySection is the object GitHub keeps the security features under, in
// both directions.
const securitySection = "security_and_analysis"

// securityStatus is how GitHub spells a feature being on and off. A status
// string rather than the boolean everything else here uses.
const (
	securityEnabled  = "enabled"
	securityDisabled = "disabled"
)

func securityField(
	name, requires string,
	want func(SettingsConfig) *bool,
	have func(CurrentSettings) FeatureState,
) settingsField {
	return settingsField{
		name:     name,
		requires: requires,
		want: func(c SettingsConfig) (any, string, bool) {
			value := want(c)
			if value == nil {
				return nil, "", false
			}

			return *value, describeBool(*value), true
		},
		have:      func(s CurrentSettings) string { return string(have(s)) },
		available: func(s CurrentSettings) bool { return have(s).Reported() },
		now:       func(s CurrentSettings) bool { return have(s) == FeatureOn },
		on:        resulting(want, func(s CurrentSettings) bool { return have(s) == FeatureOn }),
		put: func(body map[string]any, value any) {
			section, nested := body[securitySection].(map[string]any)
			if !nested {
				section = map[string]any{}
				body[securitySection] = section
			}

			enabled, _ := value.(bool)
			section[name] = map[string]any{"status": describeStatus(enabled)}
		},
	}
}

func describeStatus(enabled bool) string {
	if enabled {
		return securityEnabled
	}

	return securityDisabled
}

// merging marks one of the ways a repository may be merged.
func merging(field settingsField) settingsField {
	field.merges = true

	return field
}

func boolField(
	name string,
	want func(SettingsConfig) *bool,
	have func(CurrentSettings) bool,
) settingsField {
	return settingsField{
		name: name,
		want: func(c SettingsConfig) (any, string, bool) {
			value := want(c)
			if value == nil {
				return nil, "", false
			}

			return *value, describeBool(*value), true
		},
		have: func(s CurrentSettings) string { return describeBool(have(s)) },
		now:  have,
		on:   resulting(want, have),
		put:  flatly(name),
	}
}

// resulting answers what a setting would be left as: what this change says, or
// what the repository already has where the change says nothing.
//
// The resulting repository is what GitHub judges a dependent setting against,
// so switching a strategy on in the same request is what makes the wording sent
// beside it legal - and the same expression read the other way is what the
// repository keeps when a change is withheld.
func resulting(
	want func(SettingsConfig) *bool,
	now func(CurrentSettings) bool,
) func(SettingsConfig, CurrentSettings) bool {
	return func(c SettingsConfig, s CurrentSettings) bool {
		if value := want(c); value != nil {
			return *value
		}

		return now(s)
	}
}

func textField(
	name, requires string,
	want func(SettingsConfig) *string,
	have func(CurrentSettings) string,
) settingsField {
	return settingsField{
		name:     name,
		requires: requires,
		want: func(c SettingsConfig) (any, string, bool) {
			value := want(c)
			if value == nil {
				return nil, "", false
			}

			return *value, *value, true
		},
		have: func(s CurrentSettings) string { return have(s) },
		put:  flatly(name),
	}
}

// flatly writes a setting as a key of its own, which is where all but the
// security features live.
func flatly(name string) func(map[string]any, any) {
	return func(body map[string]any, value any) { body[name] = value }
}

// asking reports a setting configured to something that needs whatever it
// depends on.
//
// Only one direction needs anything underneath it. Turning a feature off asks
// nothing of the feature below it - GitHub refuses secret scanning being
// switched on where advanced security is off, and accepts both being switched
// off together, which is an ordinary thing for an organization to want. A
// wording is different and is a request by existing: there is no "off" to
// configure, so any value at all means wanting the strategy it belongs to.
func (f settingsField) asking(c SettingsConfig) bool {
	value, _, configured := f.want(c)

	return configured && askingValue(value)
}

// askingValue is the same question asked of a value already in hand, which is
// what the diff has by the time it needs the answer.
func askingValue(value any) bool {
	enabled, boolean := value.(bool)

	return !boolean || enabled
}

// fieldsByName addresses the table by the name GitHub knows a setting as, which
// is how a dependent setting finds the one it depends on and how a plan renders
// what a repository has now.
func fieldsByName(fields []settingsField) map[string]settingsField {
	byName := make(map[string]settingsField, len(fields))
	for _, field := range fields {
		byName[field.name] = field
	}

	return byName
}

// SettingsChange is one repository's settings that differ from configuration.
type SettingsChange struct {
	// Fields names what differs, sorted, for somebody reading the plan.
	Fields []string

	// Body is what to send, carrying only the settings configuration named.
	//
	// Only those: the endpoint replaces what it is given, so sending a value
	// for a setting nobody configured would set it from whatever the diff
	// happened to read - which is how a blind write turns "leave it alone" into
	// "make it what it was a moment ago", and loses whatever changed in between.
	Body map[string]any

	// Withheld is what this repository will not be given, because it would not
	// accept it, and why.
	//
	// Left out rather than sent and refused. GitHub answers either case with a
	// 422 on the whole request, so one repository without the feature would
	// otherwise lose every other setting in the same change - which is the
	// failure this port indicts the tool it replaces for, where one unavailable
	// feature took the rest of the run down with it. Withholding can only leave
	// a setting as it was; sending can undo the ones beside it.
	Withheld []Withholding

	// Follows names what GitHub switches off along with something in this
	// change: nobody configured it, this does not send it, and it goes off
	// anyway.
	//
	// Disabling advanced security disables what depends on it - GitHub
	// documents that and does it - so a plan naming only the setting somebody
	// typed would understate what approving it does, which is the one thing a
	// plan exists to get right. Reported rather than sent: the endpoint
	// replaces what it is given, and putting a key nobody configured into the
	// body is the rule the rest of this file is built to keep.
	Follows []string
}

// Withholding is one setting left alone, and the reason a person needs in order
// to know whether there is anything they can do about it.
type Withholding struct {
	Field  string
	Reason string
}

// The two reasons a setting is withheld. One is about this repository's plan or
// licence and cannot be configured away; the other is a setting somebody could
// turn on in the same configuration.
const (
	becauseUnavailable = "this repository does not offer it"
	becauseUnmet       = "the setting it needs is off here"

	// becauseUnmergeable is the third, and the one no configuration can be
	// checked for on its own: two merge methods off is a legal thing to ask of
	// a repository whose third one is on.
	becauseUnmergeable = "it would leave this repository no way to merge"
)

// DiffSettings reports what would have to change, and whether anything would.
//
// Only what would change: a repository where the sole difference is withheld
// gets no action at all, because there is nothing this can do about it until
// somebody configures the strategy or the repository turns it on.
func DiffSettings(config SettingsConfig, current CurrentSettings) (SettingsChange, bool) {
	change := SettingsChange{Body: map[string]any{}}
	fields := settingsFields()
	repository := judge(fields, config, current)

	// What this change switches off, which is what anything depending on it
	// goes off with.
	switchedOff := make(map[string]bool, len(fields))

	for _, field := range fields {
		value, want, configured := field.want(config)
		if !configured {
			// Not configured is not a value. Nothing is compared and nothing is
			// sent, so the repository keeps whatever it has. This is the whole
			// of that rule: there is no second place a setting can be added to
			// the body.
			continue
		}

		if want == field.have(current) {
			continue
		}

		if reason := repository.withholds(field, value); reason != "" {
			change.withhold(field.name, reason)
			repository.keeps(field, current)

			continue
		}

		change.Fields = append(change.Fields, field.name)
		field.put(change.Body, value)

		if enabled, boolean := value.(bool); boolean && !enabled {
			switchedOff[field.name] = true
		}
	}

	change.Follows = follows(fields, current, repository.absent, switchedOff)

	sort.Strings(change.Fields)
	sort.Strings(change.Follows)
	sort.Slice(change.Withheld, func(i, j int) bool {
		return change.Withheld[i].Field < change.Withheld[j].Field
	})

	return change, len(change.Fields) > 0
}

// verdict is what one repository's own state says about a change: what each
// setting would end up as, which settings it does not have at all, and whether
// it would still have a way to merge.
//
// Read once and asked many times. Every rule below needs the same three
// answers, and computing them per field is how two of them would come to
// disagree about the same repository.
type verdict struct {
	resulting map[string]bool
	absent    map[string]bool
	mergeable bool
}

func judge(fields []settingsField, config SettingsConfig, current CurrentSettings) verdict {
	answer := verdict{
		resulting: make(map[string]bool, len(fields)),
		absent:    make(map[string]bool, len(fields)),
	}

	for _, field := range fields {
		if field.on != nil {
			answer.resulting[field.name] = field.on(config, current)
		}
		if field.available != nil && !field.available(current) {
			answer.absent[field.name] = true
		}
	}

	// Whether this repository would still have a way to merge afterwards.
	//
	// Only the repository can answer it. A configuration turning two methods
	// off is legal - a repository whose third is on takes it - so the pair that
	// gets refused is that configuration meeting a repository that already had
	// the third one off, and neither half alone is enough to see it.
	answer.mergeable = !noMergeMethod(fields, func(field settingsField) (bool, bool) {
		return answer.resulting[field.name], true
	})

	return answer
}

// keeps records a setting that is not changing after all.
//
// Everything below it in the table is judged against what the repository will
// have, and a withheld change means that is what it has now rather than what
// was asked for. Without this, a chain reads its own intent: secret scanning
// withheld because advanced security is off still counted as "on" for push
// protection, which was then sent alone into the 422 that withholding exists to
// avoid. The table names a dependency before whatever depends on it, so one
// correction here is seen by everything that needs it.
func (v verdict) keeps(field settingsField, current CurrentSettings) {
	if field.now == nil {
		return
	}

	v.resulting[field.name] = field.now(current)
}

// withholds reports why a setting cannot be sent to this repository, and is
// empty where it can.
//
// One place, in the order the reasons matter. Each of them is a 422 on the
// whole request, so what is caught here is every other setting in the same
// change surviving.
func (v verdict) withholds(field settingsField, value any) string {
	switch {
	// A feature the repository does not have comes first, because nothing
	// configured here can change that: it is a plan or a licence, and every run
	// would otherwise try again and be refused again.
	case v.absent[field.name]:
		return becauseUnavailable

	// Only on the way up, and only where the dependency is something this
	// repository has. GitHub refuses a feature being switched on while what it
	// needs is off; switching it off asks nothing of anything. And a public
	// repository has secret scanning without advanced security and is told of
	// no advanced security at all, so reading that absence as "off" would
	// withhold a setting nothing was going to refuse.
	case field.requires != "" && askingValue(value) &&
		!v.resulting[field.requires] && !v.absent[field.requires]:
		return becauseUnmet

	// Every merge method the change would switch off, not an arbitrary one of
	// them: turning off one of a pair and keeping the other would leave a
	// repository half-way to a policy it can never reach, and which half
	// depended on the order of the table.
	case field.merges && !v.mergeable:
		return becauseUnmergeable

	default:
		return ""
	}
}

// follows names what GitHub switches off along with this change.
//
// Nobody configured these and nothing here sends them. GitHub disables what
// depends on a feature when that feature is disabled, so a plan that named only
// the setting somebody typed would be describing less than approving it does -
// and a plan describing less than it does is the failure the whole plan-then-
// apply split exists to prevent.
//
// Repeated until nothing more follows, because a dependency has dependants of
// its own: advanced security carries secret scanning, which carries push
// protection. One round per field is the bound rather than a condition to be
// got right, because every round that carries on names a field no round before
// it named, and there are only so many fields.
func follows(
	fields []settingsField,
	current CurrentSettings,
	absent map[string]bool,
	switchedOff map[string]bool,
) []string {
	var carried []string

	for round := 0; round < len(fields); round++ {
		spreading := false

		for _, field := range fields {
			switch {
			case field.requires == "" || !switchedOff[field.requires]:
				continue

			// Already going off by itself, already named by an earlier round,
			// or not here to go off at all. Saying it once is the point, and
			// the round that names nothing new is the one that ends the spread.
			case switchedOff[field.name] || absent[field.name]:
				continue

			// What the repository has now, not what the configuration asked
			// for. A setting already off follows nothing anywhere - and one
			// configured to what it already has is a change nothing sends, so
			// nothing else in this answer names it. That is exactly the setting
			// somebody asked to keep on and is about to lose.
			case field.now == nil || !field.now(current):
				continue
			}

			carried = append(carried, field.name)
			switchedOff[field.name] = true
			spreading = true
		}

		if !spreading {
			break
		}
	}

	return carried
}

func (c *SettingsChange) withhold(field, reason string) {
	c.Withheld = append(c.Withheld, Withholding{Field: field, Reason: reason})
}

// PlanSettings answers what one repository's settings would need.
//
// Two actions at most, and they are two because GitHub takes two requests.
// Everything the settings endpoint accepts is one action, since that endpoint
// replaces what it is given and those settings land or fail together; Dependabot
// security updates has an endpoint of its own, so it succeeds or fails on its
// own. Folding it into the other action would report it applied whenever the
// settings were, which is the one thing an action is there to get right.
func PlanSettings(
	repositoryID string,
	config SettingsConfig,
	current CurrentSettings,
) []Action {
	// Declared rather than made, the way the label and ruleset planners beside
	// this one do it: nothing appended is nil, which is what a caller reading
	// "this repository needs no work" gets from all three.
	var actions []Action

	if action, planned := planSettingsChange(repositoryID, config, current); planned {
		actions = append(actions, action)
	}

	if action, planned := planDependabot(repositoryID, config, current); planned {
		actions = append(actions, action)
	}

	return actions
}

func planSettingsChange(
	repositoryID string,
	config SettingsConfig,
	current CurrentSettings,
) (Action, bool) {
	change, differs := DiffSettings(config, current)
	if !differs {
		return Action{}, false
	}

	payload, err := json.Marshal(change.Body)
	if err != nil {
		// A map of bools and strings cannot fail to encode, and returning an
		// error would make every caller handle one that cannot happen.
		return Action{}, false
	}

	return Action{
		RepositoryID: repositoryID,
		Kind:         KindSettings,
		Operation:    OperationUpdate,

		// One subject, because GitHub replaces a repository's settings in one
		// request: they succeed or fail together, and a plan that showed them
		// as separate actions would promise an independence the API does not
		// offer.
		Subject: SettingsSubject,
		Before:  describeSettings(change.Fields, current),
		After:   describeChange(change),
		Payload: payload,
		State:   ActionPending,
	}, true
}

// planDependabot answers whether Dependabot security updates have to move.
func planDependabot(
	repositoryID string,
	config SettingsConfig,
	current CurrentSettings,
) (Action, bool) {
	want := config.DependabotSecurityUpdates
	if want == nil {
		return Action{}, false
	}

	have := current.DependabotSecurityUpdates

	// A repository that does not report the feature has nothing to switch, and
	// this is where that is decided. The request is its own, so nothing else in
	// the plan is at risk from letting it run - but it could only ever be
	// refused, and a refusal that repeats every sweep saying the same thing is
	// how a person learns to stop reading them. Left alone in silence, which is
	// what an unavailable security feature gets everywhere else here.
	if !have.Reported() {
		return Action{}, false
	}

	if (have == FeatureOn) == *want {
		return Action{}, false
	}

	payload, err := json.Marshal(DependabotChange{Enabled: *want})
	if err != nil {
		// One boolean in a struct of its own cannot fail to encode.
		return Action{}, false
	}

	return Action{
		RepositoryID: repositoryID,
		Kind:         KindSettings,
		Operation:    OperationUpdate,
		Subject:      DependabotSubject,
		Before:       string(have),
		After:        describeBool(*want),
		Payload:      payload,
		State:        ActionPending,
	}, true
}

// DependabotSubject is what a Dependabot security updates action is about.
//
// Named for the setting rather than for the repository, which is what the
// settings action beside it is called. Two actions of one kind on one
// repository are told apart by their subject, and the plan reads as the two
// requests it is going to make.
const DependabotSubject = "dependabot_security_updates"

// DependabotChange is what such an action carries.
//
// A payload rather than the operation, because the operation is "update" and
// the instruction is "on". GitHub spells the instruction as the verb - PUT to
// switch it on, DELETE to switch it off - but OperationDelete means something
// else here: it removes what configuration no longer names, and it is gated on
// removal being switched on for the kind. Reading "turn this off" out of it
// would put a security feature behind a switch meant for tidying up.
type DependabotChange struct {
	Enabled bool `json:"enabled"`
}

// DecodeDependabot reads what such an action says to apply.
func DecodeDependabot(payload []byte) (DependabotChange, error) {
	var change DependabotChange
	if err := json.Unmarshal(payload, &change); err != nil {
		return DependabotChange{}, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}

	return change, nil
}

// describeChange says what this repository is getting, and what it is not.
//
// The withheld half is named here because this is the only place a person sees
// it. A setting that is configured, differs, and is being left alone anyway is
// not something to discover from a repository that never changed.
func describeChange(change SettingsChange) string {
	parts := make([]string, 0, len(change.Withheld)+2)
	if len(change.Fields) > 0 {
		parts = append(parts, strings.Join(change.Fields, ", "))
	}

	if len(change.Follows) > 0 {
		parts = append(parts,
			"GitHub also switches off "+strings.Join(change.Follows, ", "))
	}

	for _, withheld := range change.Withheld {
		parts = append(parts, "leaving "+withheld.Field+" alone: "+withheld.Reason)
	}

	return strings.Join(parts, "; ")
}

// SettingsSubject is what a settings action is about. A repository has one set
// of them, so there is one subject and it is the same everywhere.
const SettingsSubject = "repository"

// describeSettings renders what a repository has now, for the fields about to
// change. Display only.
func describeSettings(fields []string, current CurrentSettings) string {
	byName := fieldsByName(settingsFields())

	parts := make([]string, 0, len(fields))
	for _, name := range fields {
		if field, known := byName[name]; known {
			parts = append(parts, name+"="+field.have(current))
		}
	}

	return strings.Join(parts, ", ")
}

func describeBool(value bool) string {
	if value {
		return "on"
	}

	return "off"
}

// noMergeMethod reports a repository that would be left unable to merge at all.
//
// Asked twice with two different answers to "what would this setting be": the
// configuration's own, where an unconfigured one says nothing, and the state a
// change would produce, where it says what the repository already has. The
// question is the same either way, so the walk over the table is written once.
//
// Nobody configuring any of them is not a repository that forbids merging; it
// is a repository nobody said anything about, so an answer that says nothing
// counts as neither on nor against.
func noMergeMethod(
	fields []settingsField,
	answer func(settingsField) (enabled, configured bool),
) bool {
	methods, silent := 0, 0

	for _, field := range fields {
		if !field.merges {
			continue
		}

		methods++

		enabled, configured := answer(field)
		switch {
		case enabled:
			return false
		case !configured:
			silent++
		}
	}

	return methods > 0 && silent == 0
}

// DecodeSettings reads what an action says to apply.
func DecodeSettings(payload []byte) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}

	return body, nil
}
