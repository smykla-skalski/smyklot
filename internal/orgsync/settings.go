package orgsync

import (
	"encoding/json"
	"fmt"
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
		if !contains(field.allowed, *field.value) {
			return invalid("%s must be one of %s, not %q",
				field.name, strings.Join(field.allowed, ", "), *field.value)
		}
	}

	// A repository has to allow some way of merging. GitHub refuses the last
	// one being turned off, and it refuses it as a 422 on the whole request -
	// so a configuration that turns all three off breaks every other setting
	// in the same change rather than only itself.
	if allFalse(c.AllowMergeCommit, c.AllowSquashMerge, c.AllowRebaseMerge) {
		return invalid("a repository must allow at least one way to merge")
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
}

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
	want func(SettingsConfig) (value any, display string, configured bool)
	have func(CurrentSettings) string
}

// settingsFields is every setting, in the order a person reads them.
//
// One table rather than a comparison, a request body and a description written
// separately. Those three drifting apart is how the tool this replaces came to
// validate three fields it never sent.
func settingsFields() []settingsField {
	return []settingsField{
		boolField("allow_merge_commit",
			func(c SettingsConfig) *bool { return c.AllowMergeCommit },
			func(s CurrentSettings) bool { return s.AllowMergeCommit }),
		boolField("allow_squash_merge",
			func(c SettingsConfig) *bool { return c.AllowSquashMerge },
			func(s CurrentSettings) bool { return s.AllowSquashMerge }),
		boolField("allow_rebase_merge",
			func(c SettingsConfig) *bool { return c.AllowRebaseMerge },
			func(s CurrentSettings) bool { return s.AllowRebaseMerge }),
		boolField("allow_auto_merge",
			func(c SettingsConfig) *bool { return c.AllowAutoMerge },
			func(s CurrentSettings) bool { return s.AllowAutoMerge }),
		boolField("delete_branch_on_merge",
			func(c SettingsConfig) *bool { return c.DeleteBranchOnMerge },
			func(s CurrentSettings) bool { return s.DeleteBranchOnMerge }),
		boolField("allow_update_branch",
			func(c SettingsConfig) *bool { return c.AllowUpdateBranch },
			func(s CurrentSettings) bool { return s.AllowUpdateBranch }),
		textField("squash_merge_commit_title",
			func(c SettingsConfig) *string { return c.SquashMergeCommitTitle },
			func(s CurrentSettings) string { return s.SquashMergeCommitTitle }),
		textField("squash_merge_commit_message",
			func(c SettingsConfig) *string { return c.SquashMergeCommitMessage },
			func(s CurrentSettings) string { return s.SquashMergeCommitMessage }),
		textField("merge_commit_title",
			func(c SettingsConfig) *string { return c.MergeCommitTitle },
			func(s CurrentSettings) string { return s.MergeCommitTitle }),
		textField("merge_commit_message",
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
	}
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
	}
}

func textField(
	name string,
	want func(SettingsConfig) *string,
	have func(CurrentSettings) string,
) settingsField {
	return settingsField{
		name: name,
		want: func(c SettingsConfig) (any, string, bool) {
			value := want(c)
			if value == nil {
				return nil, "", false
			}

			return *value, *value, true
		},
		have: func(s CurrentSettings) string { return have(s) },
	}
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
}

// DiffSettings reports what would have to change, and whether anything would.
func DiffSettings(config SettingsConfig, current CurrentSettings) (SettingsChange, bool) {
	change := SettingsChange{Body: map[string]any{}}

	for _, field := range settingsFields() {
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

		change.Fields = append(change.Fields, field.name)
		change.Body[field.name] = value
	}

	sort.Strings(change.Fields)

	return change, len(change.Fields) > 0
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
		After:   strings.Join(change.Fields, ", "),
		Payload: payload,
		State:   ActionPending,
	}}
}

// SettingsSubject is what a settings action is about. A repository has one set
// of them, so there is one subject and it is the same everywhere.
const SettingsSubject = "repository"

// describeSettings renders what a repository has now, for the fields about to
// change. Display only.
func describeSettings(fields []string, current CurrentSettings) string {
	byName := map[string]settingsField{}
	for _, field := range settingsFields() {
		byName[field.name] = field
	}

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

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}

	return false
}

// allFalse reports every configured value being off, ignoring the ones nobody
// configured. Three unset merge strategies is not a repository that forbids
// merging; it is a repository nobody said anything about.
func allFalse(values ...*bool) bool {
	configured := 0
	for _, value := range values {
		if value == nil {
			continue
		}
		configured++
		if *value {
			return false
		}
	}

	return configured == len(values)
}

// DecodeSettings reads what an action says to apply.
func DecodeSettings(payload []byte) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}

	return body, nil
}
