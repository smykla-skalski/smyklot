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

	// A wording no repository could ever accept. GitHub judges a commit
	// wording against the merge strategy beside it and refuses the pair as a
	// 422 on the whole request, so DiffSettings withholds one from a repository
	// that has the strategy off - which is what keeps a squash-only repository
	// from losing its whole settings change over a merge commit title. A
	// configuration that turns the strategy off itself is asking for something
	// impossible everywhere, and the place to say so is beside the field
	// somebody typed rather than in every plan, silently.

	for _, field := range fields {
		if field.requires == "" || !field.asking(c) {
			continue
		}

		strategy, known := byName[field.requires]
		if !known {
			continue
		}
		if value, _, configured := strategy.want(c); configured && value == false {
			return invalid("%s needs %s, which this configuration turns off",
				field.name, field.requires)
		}
	}

	return nil
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

// settingsFields is every setting, in the order a person reads them.
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
		on: func(c SettingsConfig, s CurrentSettings) bool {
			if value := want(c); value != nil {
				return *value
			}

			return have(s) == FeatureOn
		},
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
		on: func(c SettingsConfig, s CurrentSettings) bool {
			// What the repository would be left with: what this change says, or
			// what it already has where the change says nothing. The resulting
			// repository is what GitHub judges a dependent setting against, so
			// switching a strategy on in the same request is what makes the
			// wording sent beside it legal.
			if value := want(c); value != nil {
				return *value
			}

			return have(s)
		},
		put: flatly(name),
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

	// What each boolean would be once this change lands, which is the state
	// GitHub judges a dependent setting against, and which of them this
	// repository does not have at all.
	resulting := make(map[string]bool, len(fields))
	absent := make(map[string]bool, len(fields))

	for _, field := range fields {
		if field.on != nil {
			resulting[field.name] = field.on(config, current)
		}
		if field.available != nil && !field.available(current) {
			absent[field.name] = true
		}
	}

	// Whether this repository would still have a way to merge afterwards.
	//
	// Only the repository can answer it. A configuration turning two methods
	// off is legal - a repository whose third is on takes it - so the pair that
	// gets refused is that configuration meeting a repository that already had
	// the third one off, and nothing checked before this point has both halves.
	mergeable := !noMergeMethod(fields, func(field settingsField) (bool, bool) {
		return resulting[field.name], true
	})

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

		// A feature the repository does not have comes first, because nothing
		// configured here can change that: it is a plan or a licence, and every
		// run would otherwise try again and be refused again.
		if absent[field.name] {
			change.withhold(field.name, becauseUnavailable)

			continue
		}

		// Only on the way up, and only where the dependency is something this
		// repository has.
		//
		// GitHub refuses a feature being switched on while what it needs is
		// off; switching it off asks nothing of anything. And a dependency the
		// repository does not have is not one it is failing - a public
		// repository has secret scanning without advanced security and GitHub
		// reports no advanced security there at all, so reading that absence as
		// "off" would withhold a setting nothing was going to refuse.
		if field.requires != "" && askingValue(value) &&
			!resulting[field.requires] && !absent[field.requires] {
			change.withhold(field.name, becauseUnmet)

			continue
		}

		// Every merge method the change would switch off, not an arbitrary one
		// of them: turning off one of a pair and keeping the other would leave
		// a repository half-way to a policy it can never reach, and which half
		// depended on the order of this table.
		if field.merges && !mergeable {
			change.withhold(field.name, becauseUnmergeable)

			continue
		}

		change.Fields = append(change.Fields, field.name)
		field.put(change.Body, value)
	}

	sort.Strings(change.Fields)
	sort.Slice(change.Withheld, func(i, j int) bool {
		return change.Withheld[i].Field < change.Withheld[j].Field
	})

	return change, len(change.Fields) > 0
}

func (c *SettingsChange) withhold(field, reason string) {
	c.Withheld = append(c.Withheld, Withholding{Field: field, Reason: reason})
}

// PlanSettings answers what one repository's settings would need.
func PlanSettings(
	repositoryID string,
	config SettingsConfig,
	current CurrentSettings,
) []Action {
	change, differs := DiffSettings(config, current)
	if !differs {
		return nil
	}

	payload, err := json.Marshal(change.Body)
	if err != nil {
		// A map of bools and strings cannot fail to encode, and returning an
		// error would make every caller handle one that cannot happen.
		return nil
	}

	return []Action{{
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
	}}
}

// describeChange says what this repository is getting, and what it is not.
//
// The withheld half is named here because this is the only place a person sees
// it. A setting that is configured, differs, and is being left alone anyway is
// not something to discover from a repository that never changed.
func describeChange(change SettingsChange) string {
	parts := make([]string, 0, len(change.Withheld)+1)
	if len(change.Fields) > 0 {
		parts = append(parts, strings.Join(change.Fields, ", "))
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
