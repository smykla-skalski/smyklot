package filemerge

// RewriteTOML applies a complete desired TOML document to an existing one while
// retaining its presentation and comments outside removed values. This shares
// the same syntax-preserving writer as repository adjustments; it does not turn
// omitted desired keys into inherited keys as a merge-patch would.
func RewriteTOML(original, desired []byte) ([]byte, error) {
	base, _, err := decodeTOMLSemantic(original)
	if err != nil {
		return nil, err
	}
	merged, _, err := decodeTOMLSemantic(desired)
	if err != nil {
		return nil, err
	}
	return renderTOMLMerge(original, base, merged)
}
