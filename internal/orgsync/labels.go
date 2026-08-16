package orgsync

import (
	"fmt"
	"strings"
)

// Label is one label as configuration describes it.
type Label struct {
	// Name is the label, exactly as GitHub stores it.
	Name string `json:"name"`

	// Color is six hexadecimal digits with no leading hash, which is how the
	// API spells it.
	Color string `json:"color"`

	// Description is a pointer so that omitting it and clearing it are
	// different requests.
	//
	// The tool this replaces typed it as a string, so a configuration entry
	// that said nothing about the description sent an empty one - and wiped
	// whatever a repository had written there. There is no way to say "leave it
	// alone" with a value type, so this is not a value type.
	Description *string `json:"description,omitempty"`
}

// LabelConfig is the labels an installation expects its repositories to carry.
//
// This is the whole stored document, and the only type it decodes into. It was
// briefly two - this, and a second shape in the panel that carried the
// exclusions - and the planner decoded the one without them, so every exclusion
// somebody configured was silently ignored. One document, one type.
type LabelConfig struct {
	Labels []Label `json:"labels"`

	// AllowRemoval lets the planner propose deleting a label configuration does
	// not name. Off by default, and even on it only ever proposes: a deletion
	// reaches the plan before it reaches GitHub.
	AllowRemoval bool `json:"allow_removal"`

	// Excludes are the labels to leave alone entirely, neither created nor
	// removed. They travel with the labels because they only mean anything
	// beside them.
	Excludes []string `json:"excludes,omitempty"`
}

// Exclusions is what the planner matches against.
func (c LabelConfig) Exclusions() Excludes { return Excludes{Patterns: c.Excludes} }

// Names returns every configured label name, in configuration order.
func (c LabelConfig) Names() []string {
	names := make([]string, 0, len(c.Labels))
	for _, label := range c.Labels {
		names = append(names, label.Name)
	}

	return names
}

// maxLabelName and maxLabelDescription are GitHub's documented limits. Checking
// them here turns a 422 that would abort a repository's whole sync into a
// message beside the field somebody typed it in.
const (
	maxLabelName        = 50
	maxLabelDescription = 100
)

// Validate reports configuration GitHub would refuse, at the point somebody
// writes it rather than at the point a sweep tries to apply it.
//
// Every rule here is a failure the tool this replaces suffered at apply time,
// where a single bad entry returned 422 and abandoned every label after it on
// that repository. None of them need GitHub to answer to detect.
func (c LabelConfig) Validate() error {
	if err := c.Exclusions().Validate(); err != nil {
		return err
	}

	seen := make(map[string]string, len(c.Labels))

	for index, label := range c.Labels {
		if err := label.validate(index); err != nil {
			return err
		}

		// Case-insensitively, because GitHub is. It stores the case you give it
		// and refuses to create "Bug" alongside "bug", so a configuration
		// carrying both is one that cannot be applied - and it would fail on
		// whichever came second, differently per repository.
		folded := strings.ToLower(label.Name)
		if first, duplicate := seen[folded]; duplicate {
			if first == label.Name {
				return invalid("label %q is listed twice", label.Name)
			}

			return invalid(
				"labels %q and %q differ only in case, and GitHub treats them as one",
				first, label.Name,
			)
		}
		seen[folded] = label.Name
	}

	return nil
}

func (l Label) validate(index int) error {
	name := strings.TrimSpace(l.Name)
	if name == "" {
		return invalid("label %d has no name", index+1)
	}
	if name != l.Name {
		// GitHub trims it silently, so a configured " bug" would be created as
		// "bug" and then look missing on the next reconcile, which would create
		// it again for ever.
		return invalid("label %q has leading or trailing whitespace", l.Name)
	}
	if len(l.Name) > maxLabelName {
		return invalid("label %q is longer than %d characters", l.Name, maxLabelName)
	}

	if err := validateColor(l.Name, l.Color); err != nil {
		return err
	}

	if l.Description != nil && len(*l.Description) > maxLabelDescription {
		return invalid(
			"the description of %q is longer than %d characters", l.Name, maxLabelDescription,
		)
	}

	return nil
}

// validateColor accepts what the API accepts and nothing else.
//
// The tool this replaces passed the value through untouched, so "blue" and
// "#12345" reached GitHub, came back 422, and took every label after them on
// that repository with them.
func validateColor(name, color string) error {
	if color == "" {
		return invalid("label %q has no color", name)
	}
	if strings.HasPrefix(color, "#") {
		// Named separately from "not hexadecimal" because it is the mistake
		// somebody actually makes, having just copied the value out of a
		// stylesheet.
		return invalid("the color of %q must not start with #, GitHub wants %q", name, color[1:])
	}
	if len(color) != 6 {
		return invalid("the color of %q must be six hexadecimal digits, not %q", name, color)
	}

	for _, digit := range color {
		switch {
		case digit >= '0' && digit <= '9':
		case digit >= 'a' && digit <= 'f':
		case digit >= 'A' && digit <= 'F':
		default:
			return invalid("the color of %q is not hexadecimal: %q", name, color)
		}
	}

	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}
