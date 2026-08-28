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
	sourceFile, sourceDocument, err := parseYAMLDocument(template)
	if err != nil {
		return nil, err
	}
	file, document, err := parseYAMLDocument(template)
	if err != nil {
		return nil, err
	}

	override, err := decodeOverrides(spec.Overrides)
	if err != nil {
		return nil, err
	}

	if err := refuseMergeKeyOverride(override); err != nil {
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
		before = sourceDocument
	}

	// The merge writes and records; settle decides. What a key inherits is only
	// known once the whole file has - the override's own later keys and every
	// list rule can move it.
	edit := newMerge(document, spec.Strategy != StrategyShallow)

	if err := edit.intoMapping(document, override); err != nil {
		return nil, err
	}

	if err := edit.applyArrayRules(before, override, spec); err != nil {
		return nil, err
	}

	if err := edit.settle(); err != nil {
		return nil, err
	}

	if err := refuseDanglingAliases(document); err != nil {
		return nil, err
	}

	inheritYAMLMergePresentation(sourceDocument, document)

	return renderYAMLMerge(template, sourceFile, file)
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

// refuseDanglingAliases stops on an alias the merged document leaves unbound.
//
// A file is written with `&name` in one place and `*name` in another, and an
// override that removes the first leaves the second naming nothing - which is
// not a YAML file at all. A replacement keeps the anchor where it was, so what
// reaches here is what nothing could keep: a key deleted outright while
// something still refers to it. Refusing beats opening a pull request that
// turns somebody's workflow into a file GitHub will not load.
func refuseDanglingAliases(root *yaml.Node) error {
	return findDanglingAlias(root, map[string]struct{}{})
}

// findDanglingAlias walks the document the way it will be written out, so a
// name counts only from where it is defined.
//
// Present somewhere is not the question. An alias binds to the definition above
// it, so a document holding two of one name and losing the first leaves every
// alias between them naming nothing - a file that will not load, reported as
// written, because asking only whether the name still exists answers yes.
func findDanglingAlias(node *yaml.Node, defined map[string]struct{}) error {
	if node.Kind == yaml.AliasNode {
		if _, bound := defined[node.Value]; !bound {
			return fmt.Errorf(
				"%w: it would leave %q with no anchor above it",
				ErrUnwritable, "*"+node.Value)
		}
	}

	// Before its own content, because YAML lets a node's anchor be named from
	// inside it - `&loop [*loop]` is a document go-yaml will write.
	if node.Anchor != "" {
		defined[node.Anchor] = struct{}{}
	}

	for _, child := range node.Content {
		if err := findDanglingAlias(child, defined); err != nil {
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

// intoMapping applies an override object to a mapping node.
//
// The keys are walked in sorted order so that one override document produces
// one file, whatever order Go happened to range its map in. Which also means a
// key can be merged before the anchor it inherits from - `services` before
// `x-logging` - and is half of why nothing here decides anything until settle.
func (m *merge) intoMapping(mapping *yaml.Node, override map[string]any) error {
	for _, key := range sortedKeys(override) {
		if err := m.keyInto(mapping, key, override[key]); err != nil {
			return err
		}
	}

	return nil
}

// keyInto applies one of an override's keys to a mapping.
func (m *merge) keyInto(mapping *yaml.Node, key string, value any) error {
	if value == nil {
		return removeKey(mapping, key)
	}

	// Read only where it is read: a shallow merge looks at neither, and asking
	// walks the inheritance for an answer it then throws away.
	var (
		existing *yaml.Node
		own      = true
	)

	if m.deep {
		existing, own = keyValue(mapping, key)
	}

	nested, isObject := value.(map[string]any)

	if m.deep && isObject {
		if heldOutright(existing, own) {
			return m.intoMapping(existing, nested)
		}

		if stood := inheritedMapping(existing); stood != nil {
			return m.intoInherited(mapping, key, existing, own, stood, nested)
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

	// Written first and judged later. Whether spelling this key out says
	// anything the mapping does not already inherit cannot be answered here:
	// the override's own later keys, and every list rule, still have to run,
	// and any of them can change what this key inherits. Deciding now dropped
	// an override whose anchor the same run went on to move.
	//
	// Recorded whether or not anything is inherited here yet. A key the override
	// adds outright looks like no candidate at all, right up until a later key
	// of the same override, or a list rule, puts it on the anchor this mapping
	// reads - which is the whole reason this is judged at the end.
	if m.deep && !own {
		m.written = append(m.written, inheritedWrite{mapping: mapping, key: key})
	}

	return nil
}

// merge is one pass of an override over one document.
//
// What every step of it needs, in one place rather than threaded through every
// signature: the document, because a copy has to be given anchor names the rest
// of it is not using; whether keys are merged into or replaced; what has been
// written over an inheritance so far; and what the list rules put in.
type merge struct {
	root    *yaml.Node
	deep    bool
	written []inheritedWrite
	// The nodes the list rules attached, by identity. A copy holding one of
	// them is not settle's to take back, however redundant it looks: the rule's
	// work is not in the override settle would judge it by, so taking the copy
	// takes the work with it.
	attached map[*yaml.Node]struct{}
	// How many copies are left to rebuild, shared by every rebuild under this
	// one so the whole recursion draws on one pool. Held by pointer for that,
	// and seeded by newMerge so no merge can be built without one.
	budget *int
}

// newMerge starts a merge over a document.
func newMerge(root *yaml.Node, deep bool) *merge {
	rebuilds := rebuildBudget

	return &merge{root: root, deep: deep, budget: &rebuilds}
}

// rebuildBudget is how many copies one merge rebuilds before it starts judging
// them as they stand.
//
// Judging a copy runs a merge of its own, that merge settles, and settling
// judges the copies inside it - so the work is exponential in how deep an
// inheritance the override walks into. A template with one alias per level
// costs 2^(d+1)-1 rebuilds, and unbounded, 644 bytes at depth 22 took five and
// a half seconds while 30 would have taken about an hour.
//
// The template does not have to be deep for that. One anchor that names itself
// - 43 bytes - lets an override supply every level, so this is armed by the
// organization's file and fired by one repository's data, on a sweep with no
// deadline, one replica, and a file per repository.
//
// Bounded rather than made clever. Memoising the rebuild would need a cache of
// node trees, and cached trees are what three rounds of this went wrong on; a
// structural "does this say what it inherits" would be a third answer to a
// question the two comparators here have each got wrong twice. A count of
// rebuilds rather than a depth, because the recursive shape has depth 1.
//
// Running out is not a refusal. It falls back to judging the copy as it stands,
// which is what a write that copied nothing gets anyway - so a copy that still
// says what the inheritance says is still taken back, and only a copy gone
// stale against an anchor the same run moved is kept where a rebuild might have
// removed it. That is a flattening in a pull request somebody can read and
// close, in the one direction that never loses a repository's work.
//
// Sized so it should never be reached rather than to be tight. A rebuild costs
// around three microseconds, so this is a ceiling of about a tenth of a second
// per file written as a count - and the count is what can be bounded, because
// the same tenth of a second is a different number of rebuilds on every
// machine this runs on.
//
// The rest of that sizing is measured rather than asserted, in
// merge_internal_test.go: a workflow file spends none of this, the deepest
// inheritance any spec here writes spends one, and it first binds at sixteen
// nested aliases or an override twenty-two levels into a self-naming one. Those
// tests fail if a change to what a rebuild costs moves any of it, because a
// measurement written only in a comment is right the day it is written and
// quietly wrong afterwards.
const rebuildBudget = 65536

// rebuilding reports whether there is budget left to judge one more copy, and
// spends it where there is.
func (m *merge) rebuilding() bool {
	if m.budget == nil || *m.budget <= 0 {
		return false
	}

	*m.budget--

	return true
}

// attach records a node a list rule put into the document.
func (m *merge) attach(node *yaml.Node) {
	if m.attached == nil {
		m.attached = map[*yaml.Node]struct{}{}
	}

	m.attached[node] = struct{}{}
}

// ruleWrote reports a node a list rule attached, at this node or under it.
//
// By identity rather than by what it says. Asking whether the file changed
// under a copy cannot tell a rule's write from settle's own - the merge editing
// inside its copy read as somebody else's work and blocked the copy for ever,
// and a memory refreshed to fix that absorbed the rule's write instead and took
// the copy away with it. Who wrote is a fact about what ran, so it is recorded
// when it runs.
//
// Content only, so this terminates: an alias carries none, and a rule reaching
// through one writes into a copy standIn attaches on the way.
func (m *merge) ruleWrote(node *yaml.Node) bool {
	if node == nil || len(m.attached) == 0 {
		return false
	}

	if _, wrote := m.attached[node]; wrote {
		return true
	}

	for _, child := range node.Content {
		if m.ruleWrote(child) {
			return true
		}
	}

	return false
}

// inheritedWrite is a key the merge spelled out over one a merge key gives.
//
// Spelling it out is a change to the file, so where the merge asks for exactly
// what the mapping already inherits the key should not be there at all - the
// flattening would be the whole of the pull request. What it inherits is only
// settled once everything has run, which is why this is a record rather than a
// decision.
type inheritedWrite struct {
	mapping *yaml.Node
	key     string
	// What the key held before the write. An alias, where the mapping spelled
	// the key out - which is the other way it has a value it did not write. Nil
	// where a merge key gave the value and there was no key to displace, and
	// that difference is the whole of the remedy: a displaced alias goes back,
	// and a key that was never there comes off.
	before *yaml.Node
	// Set where the write made a copy of an inheritance, and holding the
	// override that copy was given, so it can be built again from what the
	// anchor says at the end - see rederived. Nil marks a key written outright,
	// which is not every key whose override was a mapping: an object patched
	// over a scalar replaces it and copies nothing.
	nested map[string]any
	// The names standInCopy minted for the copy, where this was one. Read by
	// sameNode so a renamed alias is not mistaken for a change.
	renamed map[string]string
}

// settle takes back every key the merge spelled out that the mapping turned out
// to inherit unchanged.
//
// Run to a fixpoint rather than once in a careful order. Taking one key off can
// make the mapping around it - or a sibling judged already - redundant in turn,
// and reasoning about the order the records happen to be in is a claim in a
// comment rather than a property of the code. Every step only ever removes,
// and a key removed stays removed, so this terminates.
func (m *merge) settle() error {
	for {
		settled := true

		// Every record every pass, rather than stopping at the first removal:
		// judging is cheap beside the rebuild a later pass would redo, and two
		// independent removals should cost two passes rather than one each.
		for _, write := range m.written {
			took, err := m.take(write)
			if err != nil {
				return err
			}

			if took {
				settled = false
			}
		}

		if settled {
			return nil
		}
	}
}

// take puts one written key back the way it was, where the mapping turns out to
// inherit what the key says, and reports whether it did.
func (m *merge) take(write inheritedWrite) (bool, error) {
	spelled, own := keyValue(write.mapping, write.key)
	if !own {
		return false, nil
	}

	// Already put back, and this is what keeps the pass monotone. A key the
	// merge deleted is gone and answered by the check above, but a key restored
	// to its alias is still there - and re-deriving over that same alias finds
	// it redundant again, every pass, for ever.
	if spelled == write.before {
		return false, nil
	}

	// Read now, not when the write was made. An alias names whatever its anchor
	// holds at the end, and a merge key gives whatever the mappings it names
	// hold at the end - which is the whole point of asking here.
	inherited := resolveAlias(write.before)
	if inherited == nil {
		inherited = resolveAlias(inheritedValue(write.mapping, write.key))
	}

	if inherited == nil {
		return false, nil
	}

	// A list rule has written here. It reaches into a copy through its own
	// path, and its work is not in the override this would judge by - so the
	// key is no longer the merge's alone to take back.
	if m.ruleWrote(spelled) {
		return false, nil
	}

	// A copy is derived from what it was copied from, so a copy taken earlier
	// says what the anchor said earlier. Built again from what the anchor says
	// now, so every key the override never named goes on following it, and only
	// the keys it did name can hold the copy in place.
	spelled, renamed, err := m.rederived(write, spelled, inherited)
	if err != nil {
		return false, err
	}

	if !sameNode(spelled, inherited, renamed) {
		return false, nil
	}

	if write.before != nil {
		// Put back, rather than through setKey, which would carry the anchor of
		// what is there now onto an alias that cannot hold one.
		write.mapping.Content[keyIndex(write.mapping, write.key)+1] = write.before

		return true, nil
	}

	deleteKey(write.mapping, write.key)

	return true, nil
}

// rederived is the copy this write would make if it ran now, for a write that
// made one. Anything else is answered with what is already there.
//
// A rebuild that cannot be made is a question that cannot be answered, and the
// answer to that is to keep what is there. The merge it runs reads the live
// document - a removal asks whether the key is inherited - so it can refuse
// where the real merge did not, and refusing here would fail a whole file over
// a copy that never appears in it.
func (m *merge) rederived(
	write inheritedWrite,
	spelled, inherited *yaml.Node,
) (*yaml.Node, map[string]string, error) {
	if write.nested == nil {
		return spelled, nil, nil
	}

	// Out of budget, so the copy is judged as it stands - which is what a write
	// that copied nothing gets, and errs only towards keeping one.
	if !m.rebuilding() {
		return spelled, write.renamed, nil
	}

	fresh, renamed := standInCopy(m.root, inherited)

	// Its own records, judged before this one: a copy is only redundant once
	// what it holds has settled. Nothing is attached, so nothing it does can be
	// seen unless this write keeps it.
	//
	// No footprint of its own, and none inherited. standInCopy copies every
	// node, so nothing a rule wrote is in here to be mistaken for the merge's -
	// and a rebuild the caller may throw away should not be able to declare
	// anything about the document either.
	//
	// The budget is shared, though, because the cost this bounds is the whole
	// recursion rather than any one merge in it.
	inner := &merge{root: m.root, deep: true, budget: m.budget}

	switch err := inner.intoMapping(fresh, write.nested); {
	case errors.Is(err, errInheritedRemoval):
		// A removal the file has since made impossible. That is an answer, not
		// a fault: what the override asks for cannot be said about the anchor
		// as it now stands, so the copy stays and says it instead. Refusing
		// here would fail a whole file over a copy that never appears in it.
		return spelled, write.renamed, nil

	case err != nil:
		return nil, nil, err
	}

	if err := inner.settle(); err != nil {
		return nil, nil, err
	}

	// Read, never written in. A list rule may have written into the copy that is
	// there, and its work is not in the override this rebuilds from - putting
	// the rebuild in would throw it away.
	return fresh, renamed, nil
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
//
// Its one caller is setNodeAt, walking a list rule's path, which always writes.
// A deep merge asks these two questions for itself and calls standInCopy
// directly, so that the answers live in one place rather than being reached at
// twice.
func standIn(root, mapping *yaml.Node, key string, existing *yaml.Node, own bool) *yaml.Node {
	stood := inheritedMapping(existing)
	if stood == nil || heldOutright(existing, own) {
		return existing
	}

	// The renaming is dropped: a list rule always writes, so there is nothing
	// to judge later and nothing to read it back through.
	copied, _ := standInCopy(root, stood)
	setKey(mapping, key, copied)

	return copied
}

// heldOutright reports the mapping's own mapping node, spelled out where it is.
// Whatever else names it means it, so a merge goes straight into it.
func heldOutright(existing *yaml.Node, own bool) bool {
	return own && existing != nil && existing.Kind == yaml.MappingNode
}

// inheritedMapping is the mapping a key stands for, or nil where it stands for
// something that is not a mapping at all.
//
// The two ways to arrive at somebody else's node, in one place because both
// write paths split on it: an alias, and a merge key. Written out twice they
// drifted apart the moment one of them grew a case.
func inheritedMapping(existing *yaml.Node) *yaml.Node {
	stood := resolveAlias(existing)
	if stood == nil || stood.Kind != yaml.MappingNode {
		return nil
	}

	return stood
}

// standInCopy is the copy a merge writes into instead of the node itself, with
// its anchors settled against the document it is about to join. Attaching it is
// the caller's.
func standInCopy(root, stood *yaml.Node) (copied *yaml.Node, renamed map[string]string) {
	copied = copyForMerge(stood)
	copied.Anchor = ""

	return copied, renameCopiedAnchors(root, copied)
}

// intoInherited merges into a copy of what a key inherits, and records it so
// settle can keep the copy only where it says something the inheritance does
// not.
//
// Standing in writes an inherited mapping out as literal keys, and that is a
// change to the file whatever the merge then does to it. Where the merge asks
// for what the mapping already gets - an empty patch, or a value the template
// already sets - the flattening is the only thing in the diff, and a pull
// request proposing it is a change nobody asked for. The override still holds
// next sync, so a template that later moves away is caught then.
//
// Attached before the merge rather than after it, because a copy made deeper in
// mints its fresh anchor names against the document, and a document that does
// not yet hold this copy is one those names can clash with. Taking it back off
// restores what the mapping held, inheritance included.
func (m *merge) intoInherited(
	mapping *yaml.Node,
	key string,
	existing *yaml.Node,
	own bool,
	stood *yaml.Node,
	nested map[string]any,
) error {
	// The copy itself, rather than through standIn: its caller has already
	// settled both questions standIn asks, and asking them again would put the
	// answer in two places. standIn keeps the one caller it is written for -
	// setNodeAt, walking a list rule's path.
	copied, renamed := standInCopy(m.root, stood)
	setKey(mapping, key, copied)

	if err := m.intoMapping(copied, nested); err != nil {
		return err
	}

	// An alias the mapping spells out is what the copy displaced, and it goes
	// back where the merge turns out to have changed nothing; a value a merge
	// key gives displaced nothing, and the key comes off instead.
	var before *yaml.Node
	if own {
		before = existing
	}

	// After the nested merge, so a key taken off inside this copy is judged
	// before the copy itself is - though settle no longer depends on that.
	m.written = append(m.written, inheritedWrite{
		mapping: mapping, key: key, before: before, nested: nested, renamed: renamed,
	})

	return nil
}

// sameWriting reports a value that would write out exactly what is already
// there, anchors and comments included.
//
// The other question, and the one a list rule asks: not "does this say the same
// thing" but "would writing it change the file". By the time a rule runs the
// deep merge has written the override's own list at that path through nodeFor,
// which keeps none of the template's item nodes - so the list there has lost
// the anchors and the comments the template wrote, and the rule's own list,
// built from clones of those items, is what puts them back. Judged by sameNode
// the two look identical and the rule declines, leaving a file with an anchor
// its aliases can no longer find and comments somebody wrote gone.
// The value's own anchor is not compared, because it is not the value's: setKey
// carries the anchor of whatever it replaces onto what it writes, so a list
// built for a rule always arrives here with none and would never match a
// template that anchors its list. That read as a change on every sweep, and the
// write it then made stood in for the inheritance - the flattening this skip
// exists to refuse. Everything under it keeps its anchor, which is the half
// this comparison was added for.
func sameWriting(one, other *yaml.Node) bool {
	return sameWritingWithin(one, other, false)
}

func sameWritingWithin(one, other *yaml.Node, anchored bool) bool {
	if one == nil || other == nil {
		return one == other
	}

	if anchored && one.Anchor != other.Anchor {
		return false
	}

	if one.HeadComment != other.HeadComment ||
		one.LineComment != other.LineComment ||
		one.FootComment != other.FootComment {
		return false
	}

	if one.Kind != other.Kind ||
		one.Tag != other.Tag ||
		one.Value != other.Value ||
		one.Style != other.Style ||
		len(one.Content) != len(other.Content) {
		return false
	}

	for at, child := range one.Content {
		if !sameWritingWithin(child, other.Content[at], true) {
			return false
		}
	}

	return true
}

// sameNode reports a copy the merge did not change, asked against what it was
// copied from.
//
// Read off the nodes rather than off what they decode to. Decoding is go-yaml
// answering as YAML 1.2, and the readers of these files are not: `restart: no`
// and `restart: "no"` decode alike and mean different things to compose, which
// is the whole reason styleFor quotes what it quotes. Comparing the decoded
// values dropped exactly those overrides and reported the sync applied. It also
// refused a template whose anchor names itself, for a merge that never needed
// to read a value at all.
//
// Written the same rather than worth the same, which is the question here:
// this decides whether to put a change in a pull request, and a scalar respelt
// is a change. `1` patched over `1.0` writes `1.0`, which is what the JSON half
// of this engine does with the same override.
//
// Anchors are ignored, and an alias is read through the renaming the copy has
// already had: standInCopy clears the copy's own anchor and renames any under
// it that the document has spoken for, repointing the copy's aliases as it
// goes. None of that is the merge changing what the mapping says, and counting
// it made the answer always "changed" for any template with an anchor and an
// alias to it inside - so the guard never fired for exactly the templates that
// have one, and an empty patch flattened a whole subtree and minted names for
// it. An alias is compared by the name it holds rather than followed: following
// it walks out of the copy, and where a template names itself it would not
// return.
func sameNode(one, other *yaml.Node, renamed map[string]string) bool {
	if one == nil || other == nil {
		return one == other
	}

	// An alias the copy carries names whatever its definition was renamed to,
	// and the one it was copied from names the original. Read through the
	// renaming rather than compared raw.
	value := one.Value
	if one.Kind == yaml.AliasNode {
		// A fresh name reaches the copy only by that rename, so finding one
		// here is enough - there is nothing to check it against.
		if was, ok := renamed[value]; ok {
			value = was
		}
	}

	// Style, not only Tag: the parser reads `no` and `"no"` alike as !!str with
	// the value "no", and quoting is the whole of the difference between them.
	// A node the merge builds carries no tag yet, so today Tag catches that
	// pair first and no spec can isolate this - it is here because the tags
	// agreeing is a property of how nodeFor happens to build a scalar, and the
	// bug it guards against is an override silently dropped.
	if one.Kind != other.Kind ||
		one.Tag != other.Tag ||
		value != other.Value ||
		one.Style != other.Style ||
		len(one.Content) != len(other.Content) {
		return false
	}

	for at, child := range one.Content {
		if !sameNode(child, other.Content[at], renamed) {
			return false
		}
	}

	return true
}

// copyForMerge copies a subtree for the merge to write into, keeping the pairs
// inside it pointing at each other.
//
// cloneNode copies each alias's pointer, which leads back into the tree the copy
// was taken from - so `img: &i alpine` beside `from: *i`, copied, gave a `from`
// that went on meaning the original's `img` however the copy's changed. What
// the template said is that the two are the same value, and a copy has to say
// that too.
//
// The anchors themselves are left alone here. Two definitions of one name is a
// document whose meaning depends on where a reader is standing, and
// renameCopiedAnchors settles that for each copy as standIn makes it - which is
// the only moment the copy's aliases can be told from the template's.
func copyForMerge(node *yaml.Node) *yaml.Node {
	within := map[*yaml.Node]*yaml.Node{}
	copied := cloneWithin(node, within)

	repointAliases(copied, within)

	return copied
}

// cloneWithin copies a subtree and records what each node was copied to.
func cloneWithin(node *yaml.Node, within map[*yaml.Node]*yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}

	copied := *node
	within[node] = &copied

	copied.Content = make([]*yaml.Node, len(node.Content))
	for index, child := range node.Content {
		copied.Content[index] = cloneWithin(child, within)
	}

	if len(node.Content) == 0 {
		copied.Content = nil
	}

	return &copied
}

// repointAliases sends an alias inside a copy at the copy's own node, where the
// thing it named was copied too. One naming something outside is left as it is:
// what it means did not move.
func repointAliases(node *yaml.Node, within map[*yaml.Node]*yaml.Node) {
	if node.Kind == yaml.AliasNode {
		if moved, copied := within[node.Alias]; copied {
			node.Alias = moved
		}
	}

	for _, child := range node.Content {
		repointAliases(child, within)
	}
}

// renameCopiedAnchors gives every anchor in a fresh copy a name the document
// does not already use, and points the copy's own aliases at the new names.
//
// Two definitions of one name is a document whose meaning depends on where the
// reader is standing: an alias binds to the nearest definition above it, so a
// copy keeping the template's anchors captures every `*name` written after it -
// a merge into one key changing values nothing addressed. Stripping them
// instead breaks the copy from the inside, because an alias the copy carries
// then names the template's node and stops following the copy's own.
//
// Renaming is the reading that keeps both. Done here rather than in one pass at
// the end, because here is the only moment the copy's aliases can be told from
// the template's: they point at the copy's nodes, and setKey will replace some
// of those nodes as the merge writes into them.
//
// Called before the copy is attached, so the document's taken names are the
// document's own.
func renameCopiedAnchors(root, copied *yaml.Node) map[string]string {
	renamed := map[string]string{}

	anchored := definitionsIn(copied)
	if len(anchored) == 0 {
		return renamed
	}

	// The document's own names, which a fresh one has to miss. A name minted
	// here goes in beside them with no nodes under it: what the map answers at
	// this point is only whether a name is spoken for.
	taken := definitionsIn(root)

	for name, nodes := range anchored {
		if _, clashes := taken[name]; !clashes {
			continue
		}

		for _, node := range nodes {
			fresh := unusedAnchor(name, taken)
			taken[fresh] = nil

			// Keyed by the fresh name, which is unique by construction. Keyed
			// by the original, a template defining one name twice keeps only
			// the last of them and every alias to the earlier one reads as a
			// change the merge did not make.
			renamed[fresh] = name

			renameAliases(copied, node, fresh)
			node.Anchor = fresh
		}
	}

	return renamed
}

// dropClonedAnchors clears the anchors on a value written beside the node it
// was cloned from, rather than over it.
//
// The other half of what renameCopiedAnchors settles, and the opposite answer,
// because the two copies point opposite ways. copyForMerge re-points a copy's
// aliases inside the copy, so its anchors are load-bearing and a clash is
// settled by renaming. cloneNode, which is what an array rule's items come
// through, deliberately leaves an alias pointing back out at the document - so
// the clone's anchors say nothing the original does not, and a second copy of
// them is a name the file now defines twice.
//
// Called only where setNodeAt reports the write did NOT take the place of what
// the path named, which is the whole of the question. Counting definitions
// instead is what two rounds of review got wrong from both ends: counting says
// "two" when the write replaced one of them, and it says "two" for a name the
// template already defined twice, where clearing either one rebinds whichever
// aliases sit between them. Where a name binds is a question about position,
// and the write site is the only place that knows.
func dropClonedAnchors(root, written *yaml.Node) {
	// Most written lists carry no anchor at all, and walking the list answers
	// that far more cheaply than walking the document.
	carried := definitionsIn(written)
	if len(carried) == 0 {
		return
	}

	// Still asked of the document, so a name nothing else defines is kept: the
	// clone is then the only definition, whatever it was written beside.
	defined := definitionsIn(root)

	for name, nodes := range carried {
		if len(defined[name]) <= 1 {
			continue
		}

		for _, node := range nodes {
			node.Anchor = ""
		}
	}
}

// definitionsIn is every anchored node under a root, keyed by the name it
// defines. A name with more than one node under it is defined more than once,
// which YAML allows and reads as the last one winning.
func definitionsIn(node *yaml.Node) map[string][]*yaml.Node {
	defined := map[string][]*yaml.Node{}
	collectDefinitions(node, defined)

	return defined
}

// collectDefinitions records every anchored node under a root, keyed by the
// name it defines.
func collectDefinitions(node *yaml.Node, into map[string][]*yaml.Node) {
	if node.Anchor != "" {
		into[node.Anchor] = append(into[node.Anchor], node)
	}

	for _, child := range node.Content {
		collectDefinitions(child, into)
	}
}

// unusedAnchor is the first name of the form `name-2`, `name-3` that nothing in
// the document has already taken.
func unusedAnchor(name string, taken map[string][]*yaml.Node) string {
	for suffix := 2; ; suffix++ {
		fresh := name + "-" + strconv.Itoa(suffix)
		if _, used := taken[fresh]; !used {
			return fresh
		}
	}
}

// renameAliases points every alias naming one definition at its new name.
//
// An alias is written out by its own Value, not by what its pointer leads to,
// so this is what actually moves it. The pointer is what says which definition
// it belongs to.
func renameAliases(node *yaml.Node, definition *yaml.Node, fresh string) {
	if node.Kind == yaml.AliasNode && node.Alias == definition {
		node.Value = fresh
	}

	for _, child := range node.Content {
		renameAliases(child, definition, fresh)
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
	if inheritedValue(mapping, key) != nil {
		return fmt.Errorf("%w: %q here comes from a %q, so %w",
			ErrUnwritable, key, mergeKey, errInheritedRemoval)
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
//
// Writing `"on":` into a workflow is the deliberate half of this. It is what
// actionlint asks for and what GitHub's own parser reads the same way, and it
// is the spelling that means one thing to every reader. Only keys an override
// adds are affected: setKey reuses the key node a template already wrote.
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
// in the document those items are going into. Where the copy has to stand on
// its own, copyForMerge is what re-points them.
func cloneNode(node *yaml.Node) *yaml.Node {
	return cloneWithin(node, map[*yaml.Node]*yaml.Node{})
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}
