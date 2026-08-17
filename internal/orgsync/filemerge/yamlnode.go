package filemerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// The tags a node built here carries. Written rather than left to the encoder,
// which would otherwise resolve a string like "1.20" back to a float on the way
// out - the whole failure this path exists to stop.
const (
	tagString = "!!str"
	tagInt    = "!!int"
	tagFloat  = "!!float"
	tagBool   = "!!bool"
	tagNull   = "!!null"
	tagSeq    = "!!seq"
	tagMap    = "!!map"
	tagMerge  = "!!merge"
)

// mergeYAML composes a YAML template with a repository's overrides, editing the
// document rather than rebuilding it.
//
// Node by node, because decoding a YAML file into Go values and encoding those
// values back does not write the file it read. Every plain scalar goes through
// the resolver on the way in and strconv on the way out, so `go-version: 1.20`
// comes back as `1.2`, `mode: 0644` as `420` (v3 keeps YAML 1.1 octals), and
// `since: 2024-01-01` as a full timestamp. Comments go, and the keys come back
// in alphabetical order. A file written that way is not the file somebody
// asked to have one key changed in.
//
// So the template's own nodes are kept and only what an override names is
// replaced. What nobody mentioned comes out byte for byte as it went in,
// comments and ordering included.
func mergeYAML(template []byte, spec Spec) ([]byte, error) {
	file, document, err := parseYAMLDocument(template)
	if err != nil {
		return nil, err
	}

	override, err := decodeOverrides(spec.Overrides)
	if err != nil {
		return nil, err
	}

	// The template as it arrived, for the list rules: the merge edits the
	// document in place, so what a list is appended to has to be taken before
	// that happens.
	//
	// Read again rather than copied. A copy of a node tree copies each alias's
	// pointer, and an alias points at its anchor in the tree it was read from -
	// so a copy followed an alias back into the document being edited, and a
	// rule appending to a list reached through one appended to what the merge
	// had already put there. Read only where a rule needs it.
	var before *yaml.Node

	if len(spec.Arrays) > 0 {
		if _, before, err = parseYAMLDocument(template); err != nil {
			return nil, err
		}
	}

	if err := mergeIntoMapping(document, override, spec.Strategy != StrategyShallow); err != nil {
		return nil, err
	}

	if err := applyYAMLArrayRules(document, before, override, spec); err != nil {
		return nil, err
	}

	return encodeYAMLDocument(file)
}

// parseYAMLDocument reads the one document a file should hold, as nodes.
//
// Both the document and the mapping inside it: the mapping is what a merge
// edits, and the document is what is written back. Writing the mapping instead
// drops whatever was attached to the document - the comment at the top of a
// file, which is where the line saying who generated it lives.
func parseYAMLDocument(template []byte) (file, root *yaml.Node, err error) {
	decoder := yaml.NewDecoder(bytes.NewReader(template))

	var document yaml.Node

	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, fmt.Errorf("%w: it holds no object", ErrUnreadable)
		}

		return nil, nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	// A decoder reads one document and stops, so a file holding two would be
	// merged as its first and written back without the rest.
	var second yaml.Node

	switch err := decoder.Decode(&second); {
	case errors.Is(err, io.EOF):

	case err != nil:
		return nil, nil, fmt.Errorf("%w: %w", ErrUnreadable, err)

	default:
		return nil, nil, fmt.Errorf("%w: %w", ErrUnreadable, errTrailingContent)
	}

	root = &document
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil, nil, fmt.Errorf("%w: it holds no object", ErrUnreadable)
		}

		root = root.Content[0]
	}

	// Merging overrides into a list or a scalar has no meaning, and answering
	// with an empty document instead would write the overrides alone over
	// whatever was there.
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%w: it holds no object", ErrUnreadable)
	}

	if err := refuseRepeatedKeys(root); err != nil {
		return nil, nil, err
	}

	unwriteMergeTags(root)

	return &document, root, nil
}

// refuseRepeatedKeys stops on a mapping that names a key twice.
//
// A reader takes the last of them, and this edits the first, so an override
// applied to such a file would be written down and then overruled by the line
// under it - the repository's own adjustment, ignored, with the file looking
// like it had been applied. YAML calls a repeated key an error; go-yaml only
// enforces that when decoding into a struct, and this reads nodes.
func refuseRepeatedKeys(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)

		for at := 0; at+1 < len(node.Content); at += 2 {
			key := node.Content[at].Value
			if _, repeated := seen[key]; repeated {
				return fmt.Errorf("%w: it names %q twice", ErrUnreadable, key)
			}

			seen[key] = struct{}{}
		}
	}

	for _, child := range node.Content {
		if err := refuseRepeatedKeys(child); err != nil {
			return err
		}
	}

	return nil
}

// unwriteMergeTags takes the resolved tag off a merge key.
//
// go-yaml reads `<<` as a scalar tagged !!merge and writes that tag back out
// explicitly, because !!merge is not one of the tags it treats as implied. The
// file then gains a `!!merge <<:` nobody wrote. Reading is unaffected either
// way - a plain `<<` is recognised as a merge key on the way in - so the tag is
// dropped and the line is written the way it arrived.
func unwriteMergeTags(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && node.Tag == tagMerge && node.Value == "<<" {
		node.Tag = ""
	}

	for _, child := range node.Content {
		unwriteMergeTags(child)
	}
}

func encodeYAMLDocument(root *yaml.Node) ([]byte, error) {
	var buffer bytes.Buffer

	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(yamlIndent)

	if err := encoder.Encode(root); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnwritable, err)
	}

	return buffer.Bytes(), nil
}

// mergeIntoMapping applies an override object to a mapping node.
//
// The keys are walked in sorted order so that one override document produces
// one file, whatever order Go happened to range its map in.
func mergeIntoMapping(mapping *yaml.Node, override map[string]any, deep bool) error {
	for _, key := range sortedKeys(override) {
		value := override[key]

		if value == nil {
			deleteKey(mapping, key)

			continue
		}

		existing := valueNode(mapping, key)

		nested, isObject := value.(map[string]any)
		if deep && isObject && existing != nil && existing.Kind == yaml.MappingNode {
			if err := mergeIntoMapping(existing, nested, true); err != nil {
				return err
			}

			continue
		}

		// An object patched onto anything that is not an object replaces it,
		// and the nulls inside the patch remove nothing because there is
		// nothing there to remove. That is what RFC 7396 says, and it is the
		// only reading that keeps a null meaning one thing. A shallow merge
		// writes what it was given, nulls and all, because it does not look
		// below the top level.
		written := value
		if deep && isObject {
			written = withoutNulls(nested)
		}

		built, err := nodeFor(written)
		if err != nil {
			return err
		}

		setKey(mapping, key, built)
	}

	return nil
}

// withoutNulls drops the keys an object patch sets to null, at every depth.
func withoutNulls(value map[string]any) map[string]any {
	kept := make(map[string]any, len(value))

	for key, nested := range value {
		switch typed := nested.(type) {
		case nil:

		case map[string]any:
			kept[key] = withoutNulls(typed)

		default:
			kept[key] = nested
		}
	}

	return kept
}

// valueNode is what a mapping records under a key, or nothing.
func valueNode(mapping *yaml.Node, key string) *yaml.Node {
	if at := keyIndex(mapping, key); at >= 0 {
		return mapping.Content[at+1]
	}

	return nil
}

// keyIndex is where a key sits in a mapping's content, which holds a key and
// its value side by side, or -1.
func keyIndex(mapping *yaml.Node, key string) int {
	for at := 0; at+1 < len(mapping.Content); at += 2 {
		if mapping.Content[at].Value == key {
			return at
		}
	}

	return -1
}

// setKey writes a value under a key, keeping the key node where there is one so
// that whatever was written beside it stays written beside it.
func setKey(mapping *yaml.Node, key string, value *yaml.Node) {
	if at := keyIndex(mapping, key); at >= 0 {
		mapping.Content[at+1] = value

		return
	}

	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tagString, Value: key}, value)
}

func deleteKey(mapping *yaml.Node, key string) {
	if at := keyIndex(mapping, key); at >= 0 {
		mapping.Content = slices.Delete(mapping.Content, at, at+2)
	}
}

// nodeFor builds a node from a value an override holds.
//
// The overrides are stored as JSON whatever the file they patch is written in,
// so a number arrives as json.Number - the digits somebody typed. They are
// written out as those digits under a numeric tag rather than converted, so an
// identifier past 2^53 lands in the file as itself.
func nodeFor(value any) (*yaml.Node, error) {
	switch typed := value.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagNull, Value: "null"}, nil

	case bool:
		return &yaml.Node{
			Kind: yaml.ScalarNode, Tag: tagBool, Value: strconv.FormatBool(typed),
		}, nil

	case string:
		// Left unstyled, so the encoder quotes a string that would otherwise
		// read back as a number, a date or a boolean, and leaves the rest bare.
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagString, Value: typed}, nil

	case json.Number:
		return &yaml.Node{
			Kind: yaml.ScalarNode, Tag: numberTag(typed), Value: typed.String(),
		}, nil

	case []any:
		return sequenceNode(typed)

	case map[string]any:
		return mappingNode(typed)

	default:
		return nil, fmt.Errorf(
			"%w: an override holds %T, which has no YAML spelling", ErrUnwritable, value)
	}
}

func numberTag(value json.Number) string {
	if strings.ContainsAny(value.String(), ".eE") {
		return tagFloat
	}

	return tagInt
}

func sequenceNode(values []any) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: tagSeq}

	for _, value := range values {
		child, err := nodeFor(value)
		if err != nil {
			return nil, err
		}

		node.Content = append(node.Content, child)
	}

	return node, nil
}

func mappingNode(values map[string]any) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: tagMap}

	for _, key := range sortedKeys(values) {
		child, err := nodeFor(values[key])
		if err != nil {
			return nil, err
		}

		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: tagString, Value: key}, child)
	}

	return node, nil
}

// cloneNode copies a node tree so that editing one does not edit the other.
//
// Not a way to take a document aside and keep it: an alias carries a pointer to
// its anchor in the tree it was read from, and this copies that pointer, so a
// copy follows an alias straight back into the original. It is used to move
// list items into a list being built, where the anchors they may name are still
// in the document those items are going into.
func cloneNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}

	copied := *node
	copied.Content = make([]*yaml.Node, len(node.Content))

	for index, child := range node.Content {
		copied.Content[index] = cloneNode(child)
	}

	if len(node.Content) == 0 {
		copied.Content = nil
	}

	return &copied
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}
