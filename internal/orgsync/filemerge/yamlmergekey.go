package filemerge

import (
	"fmt"

	yaml "go.yaml.in/yaml/v3"
)

// mergeKey is how YAML spells one mapping taking another's keys as its own.
//
// `<<: *anchor`, or `<<: [*first, *second]` for several. What a mapping spells
// out itself wins over anything a merge key gives it, and among several the
// earlier one wins. GitHub workflows, compose files and CI configuration are
// written this way constantly, so a merge engine that reads only the literal
// keys reads most of the files it exists to compose as very nearly empty.
const mergeKey = "<<"

// isMergeKey reports a key node that inherits rather than one spelled `<<`.
//
// Plain, because that is the difference YAML draws: `<<: *a` inherits and
// `"<<": *a` is a mapping with a key two characters long. Reading the second as
// inheritance is how a removal came to be refused for a key the mapping did not
// have. The tag cannot answer it here - unwriteMergeTags has already taken the
// !!merge tag off, so that the line is written back the way it arrived.
func isMergeKey(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Value == mergeKey && node.Style == 0
}

// keyValue is what a mapping holds under a key, and whether it spells that key
// out itself rather than inheriting it through a merge key.
//
// The second answer is the one that matters. A key a mapping only inherits
// lives in the mapping the anchor names, which other places in the document
// name too - so writing into it writes into all of them, and replacing it drops
// every sibling the inheritance carried.
func keyValue(mapping *yaml.Node, key string) (value *yaml.Node, own bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}

	if at := keyIndex(mapping, key); at >= 0 {
		return mapping.Content[at+1], true
	}

	return inheritedValue(mapping, key), false
}

// inheritedValue is what a mapping's merge keys give it under a key, taking no
// account of what the mapping spells out itself.
//
// Asked on its own where a removal has to know whether taking the literal key
// off would leave the mapping still holding one.
func inheritedValue(mapping *yaml.Node, key string) *yaml.Node {
	return inheritedWithin(mapping, key, map[*yaml.Node]struct{}{})
}

// inheritedWithin walks the inheritance, reading each mapping once.
//
// Once, and that is the whole of why the set is here rather than a depth
// bound. A merge key can name several mappings, so the walk branches, and a
// depth bound leaves the work exponential in the depth - `a: &a\n  <<: [*a,
// *a]` is twenty-one bytes and never returns, and a plain ladder of merge keys
// doubles per line. A mapping that has already been read contributed nothing,
// because a hit returns immediately, so meeting it again is work with a known
// answer. It also makes a cycle terminate without a bound to pick.
func inheritedWithin(
	mapping *yaml.Node,
	key string,
	seen map[*yaml.Node]struct{},
) *yaml.Node {
	if _, been := seen[mapping]; been {
		return nil
	}

	seen[mapping] = struct{}{}

	for at := 0; at+1 < len(mapping.Content); at += 2 {
		if !isMergeKey(mapping.Content[at]) {
			continue
		}

		// What a merged mapping spells out itself, and failing that what its
		// own merge keys give it. mergedMappings answers only mappings, so
		// there is nothing else to guard against here.
		for _, merged := range mergedMappings(mapping.Content[at+1]) {
			if found := keyIndex(merged, key); found >= 0 {
				return merged.Content[found+1]
			}

			if found := inheritedWithin(merged, key, seen); found != nil {
				return found
			}
		}
	}

	return nil
}

// mergedMappings is what one merge key names, in order of precedence: a
// mapping, or a list of them.
//
// Anything else is not a merge key any reader would honour, and is left to the
// literal reading rather than guessed at.
func mergedMappings(value *yaml.Node) []*yaml.Node {
	resolved := resolveAlias(value)
	if resolved == nil {
		return nil
	}

	switch resolved.Kind {
	case yaml.MappingNode:
		return []*yaml.Node{resolved}

	case yaml.SequenceNode:
		mappings := make([]*yaml.Node, 0, len(resolved.Content))

		for _, item := range resolved.Content {
			if merged := resolveAlias(item); merged != nil && merged.Kind == yaml.MappingNode {
				mappings = append(mappings, merged)
			}
		}

		return mappings

	default:
		return nil
	}
}

// refuseMergeKeyOverride stops an override that writes a merge key itself.
//
// `<<` is not a key: it is how YAML spells inheritance, and a reader takes what
// follows it as a mapping to inherit from. An override setting it writes a `<<`
// whose value is an ordinary object, string or number, and the file then fails
// to load at all - `map merge requires map or sequence of maps as the value`.
//
// Read off the override rather than caught where a mapping is merged, because
// there are four ways one reaches the file: merged into an existing mapping,
// built fresh by nodeFor, built inside an object that replaces a scalar, and
// built inside a list item. One pass over what was decoded covers every one.
func refuseMergeKeyOverride(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if key == mergeKey {
				return fmt.Errorf("%w: an override cannot set %q, which is how YAML "+
					"spells one mapping inheriting another", ErrUnwritable, mergeKey)
			}

			if err := refuseMergeKeyOverride(nested); err != nil {
				return err
			}
		}

	case []any:
		for _, item := range typed {
			if err := refuseMergeKeyOverride(item); err != nil {
				return err
			}
		}
	}

	return nil
}
