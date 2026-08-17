package filemerge

import (
	"fmt"

	yaml "go.yaml.in/yaml/v3"
)

// applyYAMLArrayRules re-merges the lists a spec names, which the merge patch
// would otherwise have replaced.
//
// The same rules the JSON path applies, over nodes: the items the template
// wrote keep the comments and the quoting they were written with, and only the
// override's own items are built fresh.
func applyYAMLArrayRules(merged, before *yaml.Node, override map[string]any, spec Spec) error {
	for _, rule := range spec.Arrays {
		// Parsed rather than trusted: Validate has already read every path, and
		// a spec reaching here unvalidated should fail rather than skip.
		keys, err := parsePath(rule.Path)
		if err != nil {
			return err
		}

		overrideValue, present := valueAt(override, keys)
		if !present {
			return fmt.Errorf(
				"%w: no override sets %s, so there is no array to %s",
				ErrNothingAddressed, rule.Path, rule.Strategy)
		}

		overrideItems, isArray := overrideValue.([]any)
		if !isArray {
			return fmt.Errorf(
				"%w: the override at %s is not a list", ErrNothingAddressed, rule.Path)
		}

		written, err := sequenceNode(overrideItems)
		if err != nil {
			return err
		}

		// The template may not carry the list at all, and that is ordinary:
		// appending to nothing is the override's own list.
		existing := nodeAt(before, keys)
		if existing != nil && existing.Kind != yaml.SequenceNode {
			existing = nil
		}

		combined, err := mergeSequences(existing, written, rule.Strategy, spec.Deduplicate)
		if err != nil {
			return err
		}

		if !setNodeAt(merged, keys, combined) {
			return fmt.Errorf(
				"%w: nothing in the merged file holds %s", ErrNothingAddressed, rule.Path)
		}
	}

	return nil
}

// mergeSequences combines the template's list with the repository's.
func mergeSequences(
	base, override *yaml.Node,
	strategy ArrayStrategy,
	deduplicate bool,
) (*yaml.Node, error) {
	combined := &yaml.Node{Kind: yaml.SequenceNode, Tag: tagSeq}

	switch strategy {
	case ArrayAppend:
		combined.Content = append(itemsOf(base), itemsOf(override)...)

	case ArrayPrepend:
		combined.Content = append(itemsOf(override), itemsOf(base)...)

	default:
		combined.Content = itemsOf(override)
	}

	if !deduplicate {
		return combined, nil
	}

	kept, err := deduplicatedNodes(combined.Content)
	if err != nil {
		return nil, err
	}

	combined.Content = kept

	return combined, nil
}

func itemsOf(sequence *yaml.Node) []*yaml.Node {
	if sequence == nil {
		return nil
	}

	items := make([]*yaml.Node, len(sequence.Content))
	for index, item := range sequence.Content {
		items[index] = cloneNode(item)
	}

	return items
}

// deduplicatedNodes keeps the first of every item that appears more than once.
//
// Compared by what the items say rather than by how they were written, so a
// template's `- read` and an override's `- "read"` are one item.
func deduplicatedNodes(items []*yaml.Node) ([]*yaml.Node, error) {
	kept := make([]*yaml.Node, 0, len(items))
	seen := make([]any, 0, len(items))

	for _, item := range items {
		value, err := decodedNode(item)
		if err != nil {
			return nil, err
		}

		if slicesContainsEqual(seen, value) {
			continue
		}

		seen = append(seen, value)
		kept = append(kept, item)
	}

	return kept, nil
}

func slicesContainsEqual(values []any, wanted any) bool {
	for _, value := range values {
		if equal(value, wanted) {
			return true
		}
	}

	return false
}

// decodedNode reads a node as a plain value, for comparing one against another.
// Nothing decoded here is ever written back.
func decodedNode(node *yaml.Node) (any, error) {
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	return value, nil
}

// nodeAt walks a path of keys into a document, following an alias and a merge
// key on the way, and changing nothing.
//
// Following both, because either is what the template says is at that path: a
// list rule addressing something reached through one found no mapping, read the
// template as carrying no list, and appended the repository's items to nothing -
// which is a replacement, silently, for a rule that says append.
func nodeAt(root *yaml.Node, keys []string) *yaml.Node {
	at := resolveAlias(root)

	for _, key := range keys {
		if at == nil || at.Kind != yaml.MappingNode {
			return nil
		}

		found, _ := keyValue(at, key)
		at = resolveAlias(found)
	}

	return at
}

// resolveAlias is what a node refers to, which is the node itself unless it is
// an alias. Bounded, because a self-referential alias would otherwise not
// return - go-yaml refuses to decode one, and this does not rely on that.
func resolveAlias(node *yaml.Node) *yaml.Node {
	for hops := 0; node != nil && node.Kind == yaml.AliasNode; hops++ {
		if hops >= mostAliasHops {
			return nil
		}

		node = node.Alias
	}

	return node
}

// mostAliasHops is how far an alias is followed before this gives up.
const mostAliasHops = 100

// setNodeAt writes a value at a path of keys, reporting whether the path was
// there to write to.
//
// Written into a copy wherever a step of the path is somebody else's - an alias,
// or a key a merge key gives the mapping. Walking with nodeAt and writing where
// it landed would have written into the anchor itself, which is to say into
// every other place in the document that names it.
func setNodeAt(root *yaml.Node, keys []string, value *yaml.Node) bool {
	if len(keys) == 0 {
		return false
	}

	parent := resolveAlias(root)

	for _, key := range keys[:len(keys)-1] {
		if parent == nil || parent.Kind != yaml.MappingNode {
			return false
		}

		found, own := keyValue(parent, key)
		parent = resolveAlias(standIn(parent, key, found, own))
	}

	if parent == nil || parent.Kind != yaml.MappingNode {
		return false
	}

	setKey(parent, keys[len(keys)-1], value)

	return true
}
