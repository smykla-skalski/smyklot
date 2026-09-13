package filemerge

import (
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/tailscale/hujson"
)

// The collection cap excludes the surrounding key and indentation. Line width
// remains a separate constraint. Zero leaves the existing automatic fit intact.
func inlineCollectionFits(characters, column int, common config.FormattingCommonPolicy) bool {
	return (common.InlineMaxChars == 0 || characters <= common.InlineMaxChars) &&
		column+characters <= common.LineWidth
}

// hujson.Clone collapses empty non-nil extras, which encode trailing commas.
// Preserve those markers before measuring the cloned collection's syntax.
func restoreJSONCommaMarkers(target, source *hujson.Value) {
	if source.AfterExtra != nil && target.AfterExtra == nil {
		target.AfterExtra = hujson.Extra{}
	}
	switch node := source.Value.(type) {
	case *hujson.Object:
		clone := target.Value.(*hujson.Object) //nolint:errcheck // Target is the structural clone of source.
		for index := range node.Members {
			restoreJSONCommaMarkers(&clone.Members[index].Value, &node.Members[index].Value)
		}
	case *hujson.Array:
		clone := target.Value.(*hujson.Array) //nolint:errcheck // Target is the structural clone of source.
		for index := range node.Elements {
			restoreJSONCommaMarkers(&clone.Elements[index], &node.Elements[index])
		}
	}
}
