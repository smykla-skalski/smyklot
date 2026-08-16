package orgsync

import (
	"fmt"
	"sort"
	"strings"
)

// CurrentLabel is a label as GitHub currently has it.
//
// Description is a value rather than a pointer here, because GitHub always
// answers with one: absent is the empty string. The asymmetry with Label is the
// point - configuration can decline to say, an observation cannot.
type CurrentLabel struct {
	Name        string
	Color       string
	Description string
}

// PlanLabels answers what would have to change for a repository's labels to
// match its configuration.
//
// Pure: it reaches nothing and returns actions rather than performing them. The
// tool this replaces computed the same diff, then applied it, then reported the
// diff's counts as the outcome - so a run that failed halfway still said it had
// created nine labels.
//
// Order is deterministic, and deliberately so. Deletions come last: the tool
// this replaces built its deletions by ranging a map, so a rename could issue
// the create before the delete and 422 against the label it was about to
// remove, differently on every run.
func PlanLabels(
	repositoryID string,
	config LabelConfig,
	current []CurrentLabel,
	exclude Excludes,
) []Action {
	have := make(map[string]CurrentLabel, len(current))
	for _, label := range current {
		have[strings.ToLower(label.Name)] = label
	}

	var (
		actions []Action
		wanted  = make(map[string]struct{}, len(config.Labels))
	)

	for _, label := range config.Labels {
		folded := strings.ToLower(label.Name)
		wanted[folded] = struct{}{}

		if exclude.Matches(label.Name) {
			continue
		}

		existing, present := have[folded]
		if !present {
			actions = append(actions, Action{
				RepositoryID: repositoryID,
				Kind:         KindLabels,
				Operation:    OperationCreate,
				Subject:      label.Name,
				After:        describeLabel(label.Name, label.Color, described(label)),
				State:        ActionPending,
			})

			continue
		}

		if after, changed := labelChange(label, existing); changed {
			actions = append(actions, Action{
				RepositoryID: repositoryID,
				Kind:         KindLabels,
				Operation:    OperationUpdate,
				Subject:      label.Name,
				Before: describeLabel(
					existing.Name, existing.Color, existing.Description,
				),
				After:  after,
				State:  ActionPending,
			})
		}
	}

	if !config.AllowRemoval {
		return actions
	}

	// Sorted, because the answer must not depend on map iteration order. Two
	// plans of the same state have to be the same plan, or a digest comparison
	// means nothing and a person reading two runs cannot tell them apart.
	surplus := make([]CurrentLabel, 0, len(current))
	for _, label := range current {
		if _, keep := wanted[strings.ToLower(label.Name)]; keep {
			continue
		}
		if exclude.Matches(label.Name) {
			continue
		}
		surplus = append(surplus, label)
	}
	sort.Slice(surplus, func(i, j int) bool { return surplus[i].Name < surplus[j].Name })

	for _, label := range surplus {
		actions = append(actions, Action{
			RepositoryID: repositoryID,
			Kind:         KindLabels,
			Operation:    OperationDelete,
			Subject:      label.Name,
			Before:       describeLabel(label.Name, label.Color, label.Description),
			State:        ActionPending,
		})
	}

	return actions
}

// labelChange reports what an existing label would become, and whether that
// differs from what it is.
//
// A configuration that says nothing about the description leaves it alone,
// which is what the pointer on Label is for. Comparing against the empty string
// instead would call every label with a description "changed" for ever, and
// then clear it.
func labelChange(want Label, have CurrentLabel) (string, bool) {
	description := have.Description
	if want.Description != nil {
		description = *want.Description
	}

	// Case-insensitively equal names can still differ in case, and GitHub keeps
	// what it was given. Renaming to the configured spelling is a real change.
	changed := want.Name != have.Name ||
		!strings.EqualFold(want.Color, have.Color) ||
		description != have.Description

	return describeLabel(want.Name, strings.ToLower(want.Color), description), changed
}

func described(label Label) string {
	if label.Description == nil {
		return ""
	}

	return *label.Description
}

// describeLabel renders a label for a person reading the plan. It is display,
// never a value anything branches on.
func describeLabel(name, color, description string) string {
	if description == "" {
		return fmt.Sprintf("%s #%s", name, color)
	}

	return fmt.Sprintf("%s #%s - %s", name, color, description)
}
