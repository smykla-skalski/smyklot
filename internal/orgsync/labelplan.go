package orgsync

import (
	"encoding/json"
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
			wanted := resolved(label, "")
			actions = append(actions, Action{
				RepositoryID: repositoryID,
				Kind:         KindLabels,
				Operation:    OperationCreate,
				Subject:      label.Name,
				After:        describeLabel(wanted),
				Payload:      encodeLabel(wanted),
				State:        ActionPending,
			})

			continue
		}

		wanted := resolved(label, existing.Description)
		if changed(wanted, existing) {
			actions = append(actions, Action{
				RepositoryID: repositoryID,
				Kind:         KindLabels,
				Operation:    OperationUpdate,
				Subject:      label.Name,
				Before:       describeLabel(ResolvedLabel(existing)),
				After:        describeLabel(wanted),
				Payload:      encodeLabel(wanted),
				State:        ActionPending,
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
			Before:       describeLabel(ResolvedLabel(label)),
			State:        ActionPending,
		})
	}

	return actions
}

// ResolvedLabel is a label with every question answered: what it is called,
// what colour it is, and what it says. It is what an action carries and what
// the executor applies, with nothing left to look up.
type ResolvedLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// DecodeLabel reads what an action says to apply.
func DecodeLabel(payload []byte) (ResolvedLabel, error) {
	var label ResolvedLabel
	if err := json.Unmarshal(payload, &label); err != nil {
		return ResolvedLabel{}, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}

	return label, nil
}

// resolved answers every question about a label, taking the description a
// repository already has where configuration declines to say.
//
// This is where "leave it alone" is turned into a value, and it happens at plan
// time on purpose: the executor sends a whole label to an endpoint that
// replaces what it is given, so somebody reading the plan has to be able to see
// the description that will be sent.
func resolved(want Label, current string) ResolvedLabel {
	description := current
	if want.Description != nil {
		description = *want.Description
	}

	return ResolvedLabel{
		Name: want.Name, Color: strings.ToLower(want.Color), Description: description,
	}
}

// changed reports a label that would differ from what the repository has.
//
// The colour case-insensitively, because GitHub answers in whatever case it
// stored and configuration is whatever somebody typed - treating those as
// different would rewrite the same label every tick for ever. The name
// case-sensitively, because GitHub keeps the case it was given and renaming to
// the configured spelling is real work.
func changed(want ResolvedLabel, have CurrentLabel) bool {
	return want.Name != have.Name ||
		!strings.EqualFold(want.Color, have.Color) ||
		want.Description != have.Description
}

func encodeLabel(label ResolvedLabel) []byte {
	// A struct of three strings cannot fail to encode, and a planner that
	// returned an error here would make every caller handle one that cannot
	// happen.
	payload, _ := json.Marshal(label)

	return payload
}

// describeLabel renders a label for a person reading the plan. It is display,
// never a value anything branches on.
func describeLabel(label ResolvedLabel) string {
	if label.Description == "" {
		return fmt.Sprintf("%s #%s", label.Name, label.Color)
	}

	return fmt.Sprintf("%s #%s - %s", label.Name, label.Color, label.Description)
}
