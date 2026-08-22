package main

import (
	"net/http"
	"path"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// schemaRoot is what the generated schemas are published under, derived from
// where the schema says it lives rather than spelled again.
//
// A route and an $id that disagree is a schema nothing can fetch, and it would
// disagree quietly: nothing here would fail, and the file in somebody's
// repository would simply stop being understood by their editor.
var schemaRoot = path.Dir(config.SchemaPath)

// schemaMaxAge is how long an editor may hold a schema before asking again.
//
// An hour rather than a year, because a version does not change what it means
// but does gain settings: a new key would otherwise be underlined as unknown in
// somebody's editor until their cache expired. Long enough that a repository
// full of TOML files costs one request, short enough that a release is visible
// the same afternoon.
const schemaMaxAge = "public, max-age=3600"

// serveSchema publishes the generated JSON Schemas.
//
// Unauthenticated, because the whole point is that an editor on a laptop that
// has never signed in can fetch it. It says nothing a repository's own
// configuration file does not already say.
func serveSchema(w http.ResponseWriter, r *http.Request) {
	document, ok := config.Schema(r.PathValue("schema"))
	if !ok {
		writeText(w, http.StatusNotFound, "no such schema\n")

		return
	}

	// The registered media type for a JSON Schema. Editors accept either, and
	// naming the specific one is what tells a human curling it what they have.
	w.Header().Set("Content-Type", "application/schema+json")

	// This route is not behind the panel's headers, and a browser that sniffs
	// its way to text/html on a document served from the same origin as the
	// panel is the whole of what nosniff exists to stop.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", schemaMaxAge)
	w.WriteHeader(http.StatusOK)

	// The request names which document, never what is in one: every byte here
	// was embedded at build time, and a name matching nothing has already been
	// answered above.
	_, _ = w.Write(document) //nolint:gosec // embedded bytes, selected by name
}
