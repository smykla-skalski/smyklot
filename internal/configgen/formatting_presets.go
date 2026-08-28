package configgen

import (
	"bytes"
	"fmt"
	"go/format"
	"strconv"
	"strings"
)

const (
	// FormattingGoFile is the backend preset implementation generated from the
	// same leaf metadata as the panel contract.
	FormattingGoFile = "pkg/config/zz_formatting_generated.go"
)

// RenderFormattingPresetsGo writes complete backend policies for every
// formatting preset. A preset's leaf values therefore have one source of truth
// across configuration resolution and the panel editor.
func RenderFormattingPresetsGo(model Model) ([]byte, error) {
	formattingField, err := formattingField(model)
	if err != nil {
		return nil, err
	}
	presets, err := formattingPresets(formattingField)
	if err != nil {
		return nil, err
	}
	presetLeaf, err := formattingPresetLeaf(formattingField)
	if err != nil {
		return nil, err
	}

	var output bytes.Buffer
	output.WriteString(goHeader)
	output.WriteString("\n\npackage config\n\n")
	output.WriteString("// formattingPresetPolicy returns the complete policy declared by preset tags.\n")
	output.WriteString("func formattingPresetPolicy(preset string) FormattingPolicy {\n")
	output.WriteString("switch preset {\n")
	for _, preset := range presets {
		if preset == presetLeaf.Default {
			continue
		}
		fmt.Fprintf(&output, "case %s:\nreturn ", strconv.Quote(preset))
		if err := writeGoPolicy(&output, formattingField, preset, 0); err != nil {
			return nil, err
		}
		output.WriteString("\n")
	}
	output.WriteString("default:\nreturn ")
	if err := writeGoPolicy(&output, formattingField, presetLeaf.Default, 0); err != nil {
		return nil, err
	}
	output.WriteString("\n}\n}\n")

	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated formatting presets: %w", err)
	}

	return formatted, nil
}

func formattingPresetLeaf(formatting Field) (Field, error) {
	for _, field := range formatting.Children {
		if field.Key == "formatting.preset" {
			return field, nil
		}
	}

	return Field{}, fmt.Errorf("%w: formatting.preset is missing", errFormattingShape)
}

func writeGoPolicy(output *bytes.Buffer, object Field, preset string, depth int) error {
	output.WriteString(object.GoType)
	output.WriteString("{\n")

	for _, field := range object.Children {
		output.WriteString(strings.Repeat("\t", depth+1))
		output.WriteString(field.GoName)
		output.WriteString(":")
		if field.Kind == KindObject {
			if err := writeGoPolicy(output, field, preset, depth+1); err != nil {
				return err
			}
		} else {
			value := field.Default
			for _, override := range field.Presets {
				if override.Name == preset {
					value = override.Value
					break
				}
			}
			if field.Kind == KindInt {
				output.WriteString(value)
			} else {
				output.WriteString(strconv.Quote(value))
			}
		}
		output.WriteString(",\n")
	}

	output.WriteString(strings.Repeat("\t", depth))
	output.WriteString("}")

	return nil
}
