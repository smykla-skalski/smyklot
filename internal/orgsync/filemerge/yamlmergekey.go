package filemerge

import (
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

// mostMergeKeyHops bounds how far inheritance is followed. A merged mapping may
// carry a merge key of its own, and a document can be written to send one back
// round to another.
const mostMergeKeyHops = 100

// keyValue is what a mapping holds under a key, and whether it spells that key
// out itself rather than inheriting it through a merge key.
//
// The second answer is the one that matters. A key a mapping only inherits
// lives in the mapping the anchor names, which other places in the document
// name too - so writing into it writes into all of them, and replacing it drops
// every sibling the inheritance carried.
func keyValue(mapping *yaml.Node, key string) (value *yaml.Node, own bool) {
	return keyValueWithin(mapping, key, 0)
}

func keyValueWithin(mapping *yaml.Node, key string, hops int) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}

	if at := keyIndex(mapping, key); at >= 0 {
		return mapping.Content[at+1], true
	}

	return inheritedValue(mapping, key, hops), false
}

// inheritedValue is what a mapping's merge keys give it under a key, taking no
// account of what the mapping spells out itself.
//
// Asked on its own where a removal has to know whether taking the literal key
// off would leave the mapping still holding one.
func inheritedValue(mapping *yaml.Node, key string, hops int) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode || hops >= mostMergeKeyHops {
		return nil
	}

	for at := 0; at+1 < len(mapping.Content); at += 2 {
		if mapping.Content[at].Value != mergeKey {
			continue
		}

		for _, merged := range mergedMappings(mapping.Content[at+1]) {
			if found, _ := keyValueWithin(merged, key, hops+1); found != nil {
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
