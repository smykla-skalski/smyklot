package storage

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// RuntimeBehavior preserves explicit field ownership independently of effective
// values. Its zero value inherits every behavior setting from the deployment.
// Runner remains deployment-owned and cannot be overridden here.
type RuntimeBehavior struct{ patch config.Patch }

// NewRuntimeBehavior validates and copies a sparse behavior override.
func NewRuntimeBehavior(patch config.Patch) (RuntimeBehavior, error) {
	if patch.Runner != nil {
		return RuntimeBehavior{}, errors.New("runtime behavior cannot override runner")
	}
	content, err := json.Marshal(patch)
	if err != nil {
		return RuntimeBehavior{}, err
	}
	decoded, err := parseRuntimeBehaviorPatch(content)
	if err != nil {
		return RuntimeBehavior{}, err
	}
	return RuntimeBehavior{patch: decoded}, nil
}

// IsEmpty reports whether every field inherits from the deployment.
func (value RuntimeBehavior) IsEmpty() bool { return len(value.patch.SetKeys()) == 0 }

// Resolve layers only explicit values onto the current deployment and returns a
// deep copy. Equal-value overrides remain pinned when deployment defaults change.
func (value RuntimeBehavior) Resolve(deployment *config.Config) *config.Config {
	return config.ApplyPatch(deployment, value.patch)
}

// Patch returns a copy suitable for the editor's per-field ownership model.
func (value RuntimeBehavior) Patch() config.Patch {
	content, _ := json.Marshal(value.patch)
	var patch config.Patch
	_ = json.Unmarshal(content, &patch)
	return patch
}

// MarshalJSON writes an envelope so sparse overrides cannot be mistaken for the
// older complete configuration stored in runtime settings and checkpoints.
func (value RuntimeBehavior) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Version   int          `json:"version"`
		Overrides config.Patch `json:"overrides"`
	}{Version: 1, Overrides: value.patch})
}

// UnmarshalJSON accepts the versioned sparse shape and historical complete
// configurations. Legacy fields stay explicit, preserving their saved behavior.
func (value *RuntimeBehavior) UnmarshalJSON(content []byte) error {
	object, err := config.DecodeJSONObject(content)
	if err != nil {
		return fmt.Errorf("runtime behavior: %w", err)
	}
	var patch config.Patch
	if version, exists := object["version"]; exists {
		if version != json.Number("1") || len(object) != 2 {
			return errors.New("runtime behavior envelope requires version 1 and overrides")
		}
		overrides, ok := object["overrides"].(map[string]any)
		if !ok {
			return errors.New("runtime behavior overrides must be an object")
		}
		raw, encodeErr := json.Marshal(overrides)
		if encodeErr != nil {
			return encodeErr
		}
		patch, err = parseRuntimeBehaviorPatch(raw)
	} else {
		// Match the historical reader's concrete Config decoding, including zero
		// values for fields absent in old records. Normalize collections to copies.
		var legacy config.Config
		// Records predating formatting must retain presentation instead of
		// producing an invalid all-zero formatting policy during migration.
		if _, exists := object["formatting"]; !exists {
			legacy.Formatting = config.DefaultFormattingPolicy()
		}
		if err = json.Unmarshal(content, &legacy); err == nil {
			if legacy.Runner != "" {
				if _, runnerErr := config.ParseRunner(string(legacy.Runner)); runnerErr != nil {
					return runnerErr
				}
			}
			patch = config.ApplyPatch(&legacy, config.Patch{}).AsPatch()
			patch.Runner = nil
			var normalized RuntimeBehavior
			normalized, err = NewRuntimeBehavior(patch)
			patch = normalized.patch
		}
	}
	if err != nil {
		return fmt.Errorf("runtime behavior: %w", err)
	}
	value.patch = patch
	return nil
}

func parseRuntimeBehaviorPatch(content []byte) (config.Patch, error) {
	object, err := config.DecodeJSONObject(content)
	if err != nil {
		return config.Patch{}, err
	}
	if err := rejectRuntimeNulls(object, ""); err != nil {
		return config.Patch{}, err
	}
	patch, err := config.ParsePatch(config.Format("json"), content)
	if err != nil {
		return config.Patch{}, err
	}
	if patch.Runner != nil {
		return config.Patch{}, errors.New("runtime behavior cannot override runner")
	}
	return patch, nil
}

func rejectRuntimeNulls(object map[string]any, path string) error {
	for key, value := range object {
		field := key
		if path != "" {
			field = path + "." + key
		}
		if value == nil {
			return &config.FieldError{Field: field, Cause: fmt.Errorf("behavior.%s must have a value or be omitted to inherit", field)}
		}
		if nested, ok := value.(map[string]any); ok {
			if err := rejectRuntimeNulls(nested, field); err != nil {
				return err
			}
		}
	}
	return nil
}
