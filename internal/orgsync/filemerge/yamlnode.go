package filemerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
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
// replaced: every value nobody mentioned comes back as it was written, with the
// comments beside it and the keys in the order they were in.
//
// Not byte for byte. The document is written out again, so a leading `---` goes,
// sequences come back at this indent whatever indent they had, and a folded
// scalar is refolded. That is a first sync landing as a whole-file reformat -
// worth knowing, and a different thing from a value that changed meaning.
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

	if err := refuseDanglingAliases(document); err != nil {
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

// refuseDanglingAliases stops on an alias whose anchor the merge took away.
//
// A file is written with `&name` in one place and `*name` in another, and an
// override that removes the first leaves the second naming nothing - which is
// not a YAML file at all. A replacement keeps the anchor where it was, so what
// reaches here is what nothing could keep: a key deleted outright while
// something still refers to it. Refusing beats opening a pull request that
// turns somebody's workflow into a file GitHub will not load.
func refuseDanglingAliases(root *yaml.Node) error {
	anchors := map[string]struct{}{}
	collectAnchors(root, anchors)

	return findDanglingAlias(root, anchors)
}

func collectAnchors(node *yaml.Node, into map[string]struct{}) {
	if node.Anchor != "" {
		into[node.Anchor] = struct{}{}
	}

	for _, child := range node.Content {
		collectAnchors(child, into)
	}
}

func findDanglingAlias(node *yaml.Node, anchors map[string]struct{}) error {
	if node.Kind == yaml.AliasNode {
		if _, held := anchors[node.Value]; !held {
			return fmt.Errorf(
				"%w: it would leave %q referring to an anchor the merge removed",
				ErrUnwritable, "*"+node.Value)
		}
	}

	for _, child := range node.Content {
		if err := findDanglingAlias(child, anchors); err != nil {
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
		if key == mergeKey {
			// Not a key: it is how YAML spells inheritance, and an override
			// setting it would write a `<<` whose value is not an anchor, which
			// is a file half the readers of these files refuse outright.
			return fmt.Errorf("%w: an override cannot set %q, which is how YAML "+
				"spells one mapping inheriting another", ErrUnwritable, mergeKey)
		}

		if err := mergeKeyInto(mapping, key, override[key], deep); err != nil {
			return err
		}
	}

	return nil
}

// mergeKeyInto applies one of an override's keys to a mapping.
func mergeKeyInto(mapping *yaml.Node, key string, value any, deep bool) error {
	if value == nil {
		return removeKey(mapping, key)
	}

	nested, isObject := value.(map[string]any)

	if deep && isObject {
		existing, own := keyValue(mapping, key)

		if stood := standIn(mapping, key, existing, own); stood != nil &&
			stood.Kind == yaml.MappingNode {
			return mergeIntoMapping(stood, nested, true)
		}

		// An object patched onto anything that is not an object replaces it,
		// and the nulls inside the patch remove nothing because there is
		// nothing there to remove. That is what RFC 7396 says, and it is the
		// only reading that keeps a null meaning one thing.
		value = withoutNulls(nested)
	}

	// A shallow merge arrives here for everything, and writes what it was given,
	// nulls and all, because it does not look below the top level.
	built, err := nodeFor(value)
	if err != nil {
		return err
	}

	setKey(mapping, key, built)

	return nil
}

// standIn writes out what a mapping does not hold itself, so a deep merge has a
// mapping of its own to merge into, and answers with what to merge into.
//
// Two ways to arrive at somebody else's node. An alias stands for the mapping it
// names: read as the node it literally is, it is not a mapping at all, so a deep
// merge replaced the whole thing and every key the alias carried went with it. A
// merge key is the same thing spelled differently - the mapping inherits keys it
// does not spell out, and reading only the literal ones finds them absent.
//
// A copy in either case, put where the merge can see it, rather than the node
// itself: what an anchor names is shared, and merging into it would change every
// other place naming it.
func standIn(mapping *yaml.Node, key string, existing *yaml.Node, own bool) *yaml.Node {
	stood := resolveAlias(existing)
	if stood == nil || stood.Kind != yaml.MappingNode {
		return existing
	}

	// The mapping's own mapping node, spelled out where it is. Nothing else
	// names it, so the merge goes straight into it.
	if own && existing.Kind == yaml.MappingNode {
		return existing
	}

	copied := cloneNode(stood)
	forgetAnchors(copied)
	setKey(mapping, key, copied)

	return copied
}

// forgetAnchors strips the anchors from a copy, at every depth.
//
// One that kept them would define them a second time, and an alias binds to the
// nearest definition above it - so every `*name` written after the copy would
// name the copy rather than what the template anchored, and a merge into one key
// would change values nothing addressed. Clearing the copied node alone was not
// enough: an anchor nested inside it does the same thing one level down.
//
// The aliases inside the copy still resolve. What they name is defined where it
// always was, above the copy, and that is also what they meant before the merge.
func forgetAnchors(node *yaml.Node) {
	if node == nil {
		return
	}

	node.Anchor = ""

	// Content only, never Alias: that pointer leads out of the copy and into the
	// document the copy was taken from, where the anchors have to stay.
	for _, child := range node.Content {
		forgetAnchors(child)
	}
}

// removeKey takes a key off a mapping, refusing where the mapping would go on
// holding it.
//
// A key that arrives through a merge key is not this mapping's to remove. It
// lives in the mapping the anchor names, which other places name too, and
// unpicking the inheritance - writing out every key it carries but this one -
// would leave a mapping that no longer follows the anchor it was written to
// follow, which is a change nobody asked for.
//
// Removing the literal key is worse still where a merge key gives the same one:
// the key does not go, it goes back to what was inherited, so a removal lands as
// a change of value.
func removeKey(mapping *yaml.Node, key string) error {
	if inheritedValue(mapping, key, 0) != nil {
		return fmt.Errorf(
			"%w: %q here comes from a %q, so removing it would mean unpicking "+
				"what this mapping inherits", ErrUnwritable, key, mergeKey)
	}

	deleteKey(mapping, key)

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
		// The anchor belongs to the place rather than to the value that was
		// there: aliases elsewhere in the file name it, and a replacement that
		// dropped it left them naming nothing - which is a file that does not
		// parse, written into somebody's repository by a bot.
		value.Anchor = mapping.Content[at+1].Anchor
		mapping.Content[at+1] = value

		return
	}

	mapping.Content = append(mapping.Content, keyNode(key), value)
}

// keyNode is a key written fresh into a mapping.
//
// Quoted by the same rule the values are, and for the same reason: a key of
// `on`, `no`, `off` or `12:30` written bare comes back from a YAML 1.1 reader as
// `true`, `false`, `false` and `750`. That reader is compose, PyYAML, actionlint
// or yamllint, and the key it mangles is the one that decides when a workflow
// runs - `on:` is the commonest key in the files this synchronizes.
func keyNode(key string) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.ScalarNode, Tag: tagString, Value: key, Style: styleFor(key),
	}
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
		// Mostly left unstyled, so the encoder quotes what would otherwise read
		// back as a number or a date and leaves the rest bare. It decides that
		// against YAML 1.2, which is not what reads these files: a repository
		// setting `restart: "no"` in a compose file got `restart: no`, and
		// compose reads YAML 1.1, where that is false.
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   tagString,
			Style: styleFor(typed),
			Value: typed,
		}, nil

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

// oldBooleans are the words YAML 1.1 reads as true or false, which YAML 1.2
// dropped and go-yaml therefore writes bare. The readers of the files this
// synchronizes have not all moved: compose, PyYAML and a good deal of CI still
// read the old spelling, so a string that is one of these is quoted.
var oldBooleans = map[string]struct{}{
	"y": {}, "n": {}, "yes": {}, "no": {}, "on": {}, "off": {},
}

// sexagesimal is the other thing YAML 1.1 reads and 1.2 does not: `12:30` as
// the number 750.
var sexagesimal = regexp.MustCompile(`^[-+]?[0-9][0-9_]*(:[0-5]?[0-9])+$`)

// styleFor says how a string has to be written to come back as that string.
func styleFor(value string) yaml.Style {
	if _, old := oldBooleans[strings.ToLower(value)]; old || sexagesimal.MatchString(value) {
		return yaml.DoubleQuotedStyle
	}

	// Everything else is left to the encoder, which quotes what YAML 1.2 would
	// resolve - numbers, dates, true, false, null and the rest.
	return 0
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

		node.Content = append(node.Content, keyNode(key), child)
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
