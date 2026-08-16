package config

import "embed"

// schemas is every generated JSON Schema, embedded rather than read from disk.
//
// The published bytes are then provably the bytes this binary decodes against,
// which is the property dotsync's schema lacked: its lived on a branch of
// another repository and came to describe types that had moved on. A copy that
// can be edited without rebuilding is a copy that will be.
//
//go:embed schema/*.json
var schemas embed.FS

// Schema returns a generated schema document by file name, and reports whether
// there is one.
//
// By name rather than one accessor per document, because the service publishes
// whatever is generated: a schema added here is served without anything having
// to be told about it.
func Schema(name string) ([]byte, bool) {
	// ReadFile refuses a name that is not a valid path, so a request for
	// "../../etc/passwd" is an unknown schema rather than a traversal. The
	// router will not produce one, and this does not depend on that.
	content, err := schemas.ReadFile("schema/" + name)

	return content, err == nil
}
