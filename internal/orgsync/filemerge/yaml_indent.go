package filemerge

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/goccy/go-yaml/ast"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func reindentYAMLSequences(content []byte, policy config.FormattingPolicy) ([]byte, error) {
	if policy.YAML.SequenceIndent == formatPreserve {
		return content, nil
	}
	file, err := parseGoccyYAML(content)
	if err != nil {
		return nil, err
	}
	tree := yamlSourceTree{}
	collectYAMLSourceNode(file.Docs[0].Body, yamlCollectionRef{root: true}, &tree)
	lineDeltas := yamlSequenceLineDeltas(content, tree.collections, policy)
	if len(lineDeltas) == 0 {
		return content, nil
	}

	edits, err := yamlIndentEdits(content, lineDeltas)
	if err != nil {
		return nil, err
	}

	return applyByteEdits(content, edits)
}

func yamlSequenceLineDeltas(
	content []byte,
	collections []yamlCollectionRef,
	policy config.FormattingPolicy,
) map[int]int {
	indentWidth := policy.Common.IndentWidth
	if policy.Common.IndentStyle == formatPreserve {
		indentWidth = yamlIndentWidth(content, indentWidth)
	}
	lineDeltas := make(map[int]int)
	for _, ref := range collections {
		sequence, ok := ref.source.(*ast.SequenceNode)
		if !ok || sequence.IsFlowStyle || ref.mappingValue == nil {
			continue
		}
		current := sequence.Start.Position.Column - 1
		keyIndent := ref.mappingValue.Key.GetToken().Position.Column - 1
		target := keyIndent
		if policy.YAML.SequenceIndent == "indented" {
			target += indentWidth
		}
		delta := target - current
		if delta == 0 {
			continue
		}
		for line := sequence.Start.Position.Line; line <= yamlNodeLastLine(sequence); line++ {
			lineDeltas[line] += delta
		}
	}

	return lineDeltas
}

func yamlIndentEdits(content []byte, lineDeltas map[int]int) ([]byteEdit, error) {
	layout := newSourceLayout(content)
	lines := make([]int, 0, len(lineDeltas))
	for line := range lineDeltas {
		lines = append(lines, line)
	}
	slices.Sort(lines)
	edits := make([]byteEdit, 0, len(lines))
	for _, line := range lines {
		entry, err := layout.line(line)
		if err != nil {
			return nil, err
		}
		body := content[entry.start:entry.contentEnd]
		leading := len(body) - len(bytes.TrimLeft(body, " "))
		if len(bytes.TrimSpace(body)) == 0 {
			continue
		}
		target := leading + lineDeltas[line]
		if target < 0 || leading < -lineDeltas[line] {
			return nil, fmt.Errorf(
				"%w: YAML sequence indentation would move line %d before its parent",
				ErrUnwritable, line,
			)
		}
		edits = append(edits, byteEdit{
			start:       entry.start,
			end:         entry.start + leading,
			replacement: bytes.Repeat([]byte{' '}, target),
		})
	}

	return edits, nil
}
