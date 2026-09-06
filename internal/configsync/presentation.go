package configsync

import "github.com/pelletier/go-toml/v2"

// Preserve accepted spellings whose meaning has not changed. Durations, omitted
// optional lists and JSON held in TOML strings all have a richer presentation
// than the normalized documents used for comparison.
func preserveFilePresentation(original, desired, before, after []byte) ([]byte, error) {
	var oldWire, nextWire map[string]any
	if err := toml.Unmarshal(original, &oldWire); err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(desired, &nextWire); err != nil {
		return nil, err
	}
	oldMeaning, err := decode(before)
	if err != nil {
		return nil, err
	}
	nextMeaning, err := decode(after)
	if err != nil {
		return nil, err
	}
	oldObject, _ := oldMeaning.(map[string]any)
	nextObject, _ := nextMeaning.(map[string]any)
	retainEquivalentWire(oldWire, nextWire, oldObject, nextObject)
	return toml.Marshal(nextWire)
}

func retainEquivalentWire(before, after, oldMeaning, nextMeaning map[string]any) {
	for key, value := range after {
		if sameSlot(at(oldMeaning, key), at(nextMeaning, key)) {
			if original, exists := before[key]; exists {
				after[key] = original
			} else {
				delete(after, key)
			}
			continue
		}
		nextObject, nested := value.(map[string]any)
		if !nested {
			continue
		}
		oldObject, _ := before[key].(map[string]any)
		oldSemantic, _ := oldMeaning[key].(map[string]any)
		nextSemantic, _ := nextMeaning[key].(map[string]any)
		retainEquivalentWire(oldObject, nextObject, oldSemantic, nextSemantic)
	}
}
