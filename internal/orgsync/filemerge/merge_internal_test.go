package filemerge

import (
	"testing"
)

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
