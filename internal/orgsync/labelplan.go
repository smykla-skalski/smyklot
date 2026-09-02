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
				Payload:      encodeLabelChange(wanted, ResolvedLabel(existing)),
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
			// A removal carries its label too. Apply does not read it - there is
			// nothing to write - but every other label action has one, and a
			// reader that has to render the label it is losing should not be the
			// one action that cannot say what colour it was.
			Payload: encodeLabel(ResolvedLabel(label)),
			State:   ActionPending,
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
	plan, err := DecodeLabelPlan(payload)
	if err != nil {
		return ResolvedLabel{}, err
	}

	return plan.Label, nil
}

// LabelPlan is a label action's payload: the label to write, and the one it
// replaces where there is one.
//
// The label used to be the whole payload. It is a field now because a CHANGE
// has two sides and only one was carried: the plan page could draw the label a
// repository would end up with, and had nothing to say about the one it has -
// so a colour drifting from red to orange read as `bug` twice with no
// difference between them.
type LabelPlan struct {
	// Label is what to write, and the only part apply reads.
	Label ResolvedLabel `json:"label"`

	// Previous is what the repository holds now, on an update. Nil on a
	// creation, which replaces nothing, and on a deletion, where the label
	// being removed is the one in Label.
	Previous *ResolvedLabel `json:"previous,omitempty"`
}

// DecodeLabelPlan reads a label payload in either shape it can have.
//
// A plan lives in the store for hours, so a deploy straddles one: a payload
// written before the label became a field is a bare label, and it still has to
// apply. A wrapper always names `label`, and a bare label never has a key by
// that name - a label has `name`, `color` and `description` - so the two are
// told apart by asking rather than by version.
func DecodeLabelPlan(payload []byte) (LabelPlan, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return LabelPlan{}, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}

	if _, wrapped := raw["label"]; !wrapped {
		var label ResolvedLabel
		if err := json.Unmarshal(payload, &label); err != nil {
			return LabelPlan{}, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
		}

		return LabelPlan{Label: label}, nil
	}

	var plan LabelPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return LabelPlan{}, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}

	return plan, nil
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
	return encodeLabelPlan(LabelPlan{Label: label})
}

// encodeLabelChange carries both sides, for an update: what a repository holds
// and what it would hold.
func encodeLabelChange(label, previous ResolvedLabel) []byte {
	return encodeLabelPlan(LabelPlan{Label: label, Previous: &previous})
}

func encodeLabelPlan(plan LabelPlan) []byte {
	// Strings in structs cannot fail to encode, and a planner that returned an
	// error here would make every caller handle one that cannot happen.
	payload, _ := json.Marshal(plan)

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
