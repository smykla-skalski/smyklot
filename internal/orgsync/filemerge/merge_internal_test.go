package filemerge

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestTheRebuildBudgetIsNotReachedByAnythingReal is the evidence behind the
// number rebuildBudget is set to.
//
// Its doc comment cites measurements, and a measurement in a comment is a claim
// nobody can re-run: it is right when it is written and silently wrong after
// the next change to how much work a rebuild is. This runs them.
//
// Internal because what is being measured is the budget itself. Exporting a
// counter to prove a constant would be production surface that exists for a
// test, and the merge is right here.
func TestTheRebuildBudgetIsNotReachedByAnythingReal(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]struct {
		template string
		override string
		most     int
	}{
		// What this actually syncs. A workflow has no inheritance in it, so
		// there is nothing to judge and nothing to rebuild.
		"a workflow file": {
			template: "name: Test\non:\n  push:\n    branches: [main]\njobs:\n" +
				"  test:\n    runs-on: ubuntu-latest\n    steps:\n" +
				"      - uses: actions/checkout@v5\n      - run: mise run test\n",
			override: `{"on": {"push": {"branches": ["main", "release"]}}}`,
			most:     0,
		},

		// A merge key onto an anchor with a nested one inside it, which is the
		// deepest inheritance any spec in this package writes.
		"a template that does inherit": {
			template: "oth: &o\n  k: v\nbase: &b\n  inner:\n    <<: *o\nthing:\n  <<: *b\n",
			override: `{"thing": {"inner": {"k": "v"}}}`,
			most:     1,
		},

		// Two chains beside each other, each judged on its own. The cost of a
		// wide document is the sum of its chains rather than the product, which
		// is the whole reason depth is the thing worth bounding.
		"two inheritances side by side": {
			template: "oth: &o\n  k: v\nbase: &b\n  inner:\n    <<: *o\n" +
				"one:\n  <<: *b\ntwo:\n  <<: *b\n",
			override: `{"one": {"inner": {"k": "v"}}, "two": {"inner": {"k": "v"}}}`,
			most:     2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if spent := rebuildsFor(t, document.template, document.override); spent != document.most {
				t.Errorf("spent %d rebuilds, expected %d - if this went up, "+
					"rebuildBudget's comment is now wrong", spent, document.most)
			}
		})
	}
}

// TestTheRebuildBudgetBindsWhereItSaysItDoes pins the two depths the constant's
// comment names, so a change to the cost of a rebuild cannot quietly move them.
func TestTheRebuildBudgetBindsWhereItSaysItDoes(t *testing.T) {
	t.Parallel()

	for name, shape := range map[string]struct {
		build func(int) (string, string)
		binds int
	}{
		// One alias per level, so the organization's own file carries the depth.
		"nested aliases": {build: ladderDocument, binds: 16},

		// An anchor that names itself, where the override carries all of it.
		"a self-naming anchor": {build: recursiveDocument, binds: 22},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			template, override := shape.build(shape.binds - 1)
			if spent := rebuildsFor(t, template, override); spent >= rebuildBudget {
				t.Errorf("the budget already binds one level shallower than %d", shape.binds)
			}

			template, override = shape.build(shape.binds)
			if spent := rebuildsFor(t, template, override); spent < rebuildBudget {
				t.Errorf("the budget does not bind at %d, it spent %d", shape.binds, spent)
			}
		})
	}
}

// TestTheRebuildBudgetStopsDepthMattering is the bound itself, asserted without
// a clock.
//
// Bounded and exponential are three orders of magnitude apart in seconds, so a
// deadline can say which one this is - but only loosely, because a threshold
// tight enough to measure anything fails when the machine is busy. The work is
// the thing that is actually bounded, and doubling the depth is the sharpest
// question to ask of it: capped, the answer does not move at all; uncapped, the
// second number is 2^40 times the first.
func TestTheRebuildBudgetStopsDepthMattering(t *testing.T) {
	t.Parallel()

	deep, deeper := rebuildsAt(t, 40), rebuildsAt(t, 80)

	if deep != deeper {
		t.Errorf("doubling the depth changed the work from %d to %d, "+
			"so something is no longer bounded by the budget", deep, deeper)
	}

	if deep != rebuildBudget {
		t.Errorf("spent %d of the budget at depth 40, expected all %d of it - "+
			"if this fell, the shape stopped reaching the bound and proves nothing",
			deep, rebuildBudget)
	}
}

func rebuildsAt(t *testing.T, depth int) int {
	t.Helper()

	template, override := recursiveDocument(depth)

	return rebuildsFor(t, template, override)
}

// rebuildsFor merges a document and answers how much of the budget it spent.
//
// The steps of mergeYAML rather than a call to it, because what is wanted is
// the merge afterwards and mergeYAML answers with bytes.
func rebuildsFor(t *testing.T, template, override string) int {
	t.Helper()

	_, document, err := parseYAMLDocument([]byte(template))
	if err != nil {
		t.Fatalf("reading the template: %v", err)
	}

	patch, err := decodeOverrides(json.RawMessage(override))
	if err != nil {
		t.Fatalf("reading the override: %v", err)
	}

	edit := newMerge(document, true)

	if err := edit.intoMapping(document, patch); err != nil {
		t.Fatalf("merging: %v", err)
	}

	if err := edit.settle(); err != nil {
		t.Fatalf("settling: %v", err)
	}

	return rebuildBudget - *edit.budget
}

// ladderDocument is a template of one alias per level, and an override that
// asks each of them for exactly what it already gives.
func ladderDocument(depth int) (template, override string) {
	var written strings.Builder

	written.WriteString("lvl0: &lvl0\n  leaf: v\n")

	for at := 1; at <= depth; at++ {
		fmt.Fprintf(&written, "lvl%d: &lvl%d\n  down: *lvl%d\n", at, at, at-1)
	}

	fmt.Fprintf(&written, "top: *lvl%d\n", depth)

	return written.String(), `{"top": ` + strings.Repeat(`{"down": `, depth) +
		`{"leaf": "v"}` + strings.Repeat("}", depth) + `}`
}

// recursiveDocument is forty-three bytes whatever the depth: the override walks
// into an anchor that names itself.
func recursiveDocument(depth int) (template, override string) {
	return "a: &a\n  self: *a\n  leaf: base\ntop:\n  k: *a\n",
		`{"top": {"k": ` + strings.Repeat(`{"self": `, depth) +
			`{"leaf": "base"}` + strings.Repeat("}", depth) + `}}`
}

// TestAMergedDocumentSharesNothingWithWhatItWasBuiltFrom pins the property that
// makes a merge safe to run more than once.
//
// The engine this replaces returned a document whose untouched branches were
// the caller's own maps, and then wrote array strategies through them - so
// merging one template for a second repository saw what the first repository's
// merge had left behind. Apply reaches this only with documents it parsed a
// moment earlier, so the entanglement is invisible from outside; it is the
// function's own invariant, and it is checked here rather than assumed.
func TestAMergedDocumentSharesNothingWithWhatItWasBuiltFrom(t *testing.T) {
	t.Parallel()

	for name, merge := range map[string]func(base, patch map[string]any) map[string]any{
		"deep":    mergePatch,
		"shallow": mergeShallow,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			base := map[string]any{
				"nested": map[string]any{"list": []any{"kept"}},
			}
			patch := map[string]any{
				"other": map[string]any{"list": []any{"added"}},
			}

			merged := merge(base, patch)

			write(merged, "nested", "overwritten")
			write(merged, "other", "overwritten")

			if got := read(base, "nested"); got != "kept" {
				t.Errorf("writing to the merged document changed the base: %v", got)
			}

			if got := read(patch, "other"); got != "added" {
				t.Errorf("writing to the merged document changed the patch: %v", got)
			}
		})
	}
}

func write(document map[string]any, key, value string) {
	nested, ok := document[key].(map[string]any)
	if !ok {
		return
	}

	list, ok := nested["list"].([]any)
	if !ok || len(list) == 0 {
		return
	}

	list[0] = value
}

func read(document map[string]any, key string) any {
	nested, ok := document[key].(map[string]any)
	if !ok {
		return nil
	}

	list, ok := nested["list"].([]any)
	if !ok || len(list) == 0 {
		return nil
	}

	return list[0]
}
