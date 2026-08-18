package filemerge

import (
	"fmt"

	yaml "go.yaml.in/yaml/v3"
)

// applyArrayRules re-merges the lists a spec names, which the merge patch would
// otherwise have replaced.
//
// The same rules the JSON path applies, over nodes: the items the template
// wrote keep the comments and the quoting they were written with, and only the
// override's own items are built fresh.
//
// A method rather than a pass beside the merge, because it is the merge's only
// other writer and settle has to be able to tell the two apart. Every node this
// puts into the document is declared as it goes, so that question is answered
// by what happened rather than inferred afterwards from the shape of the file.
func (m *merge) applyArrayRules(before *yaml.Node, override map[string]any, spec Spec) error {
	for _, rule := range spec.Arrays {
		// Read rather than trusted: Validate has already read every path, and a
		// spec reaching here unvalidated should fail rather than skip.
		keys, overrideItems, err := overrideListFor(override, rule, ErrNothingAddressed)
		if err != nil {
			return err
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

		// A rule whose result is what the file already says writes nothing.
		// Writing it anyway would stand in for an inherited list - flattening
		// the inheritance out as literal keys, which is the whole diff, for a
		// rule that changed no value.
		if sameWriting(nodeAt(m.root, keys), combined) {
			continue
		}

		if !m.setNodeAt(keys, combined) {
			return fmt.Errorf(
				"%w: nothing in the merged file holds %s", ErrNothingAddressed, rule.Path)
		}

		// Here rather than inside setNodeAt, which writes what it is handed and
		// has no reason to know where it came from. This list is part clone of
		// the template's, so the anchors needing settling are the ones the
		// clone carried, and this is where that is known.
		if !spelledOutAt(before, keys) {
			dropClonedAnchors(m.root, combined)
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

		if holdsEqual(seen, value) {
			continue
		}

		seen = append(seen, value)
		kept = append(kept, item)
	}

	return kept, nil
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
//
// Declaring the value at the end declares the whole path. A copy standIn makes
// on the way is attached to hold this write and nothing else, so the value ends
// up inside it - and settle asks whether a rule wrote anywhere under a key, not
// at it. Declaring those copies as well would be a second way of saying the
// same thing, with no case that needs it.
func (m *merge) setNodeAt(keys []string, value *yaml.Node) bool {
	if len(keys) == 0 {
		return false
	}

	parent := resolveAlias(m.root)

	for _, key := range keys[:len(keys)-1] {
		if parent == nil || parent.Kind != yaml.MappingNode {
			return false
		}

		found, own := keyValue(parent, key)
		parent = resolveAlias(standIn(m.root, parent, key, found, own))
	}

	if parent == nil || parent.Kind != yaml.MappingNode {
		return false
	}

	setKey(parent, keys[len(keys)-1], value)
	m.attach(value)

	return true
}

// spelledOutAt reports a path the template writes out itself, every step of it,
// with no alias and no merge key anywhere along the way.
//
// This is the one question the written list's anchors turn on. A path spelled
// out is a node the write takes the PLACE of, so the items cloned from it carry
// the only definitions of their names left and have to keep them. A path that
// goes through an alias or a merge key at any step names somebody else's node,
// which the write leaves exactly where it was - so the clone's anchors are a
// second definition of a name the document still has.
//
// Asked of the template rather than of the document being written, because by
// the time the list rules run the merge has already written its own keys, and
// a key it added looks from the merged document exactly like one the template
// spelled out. That is what made this read as a replacement and keep a
// duplicate. Counting definitions instead is what two rounds of review got
// wrong from both ends: the count says two when the write replaced one of them,
// and it says two for a name the template itself defines twice - where clearing
// either one silently rebinds whichever aliases sit between them.
func spelledOutAt(before *yaml.Node, keys []string) bool {
	at := resolveAlias(before)
	if at == nil || at.Kind == yaml.AliasNode {
		return false
	}

	for index, key := range keys {
		if at.Kind != yaml.MappingNode {
			return false
		}

		value, own := keyValue(at, key)
		if !own || value == nil || value.Kind == yaml.AliasNode {
			return false
		}

		if index == len(keys)-1 {
			return true
		}

		at = value
	}

	return false
}
