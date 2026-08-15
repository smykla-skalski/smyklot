package panel

import (
	"encoding/json"
	"html"
	"net/http"
	"strconv"
	"strings"
)

const (
	errorSentinel    = "__smyklot_panel_error__"
	noscriptSentinel = "__smyklot_panel_noscript__"
	defaultNoscript  = "The Smyklot panel needs JavaScript to run."
)

// writePageError answers with whatever the caller can read: the panel's own error
// page for a browser that navigated here, and the API's JSON for everything else.
//
// Only the handlers a reader can reach by typing or following a link use it - the
// sign-in round trip and the asset route. Everything under /api/v1 is called by
// fetch, which wants the JSON body it already parses, so those keep writeError.
func (s *Server) writePageError(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code, message string,
) {
	if !wantsDocument(r) {
		writeError(w, status, code, message)

		return
	}
	s.writeErrorDocument(w, status, code, message)
}

// writePageInternal is writeInternal for a navigable route: it says as little as
// writeInternal does, and says it in the page rather than in JSON.
func (s *Server) writePageInternal(w http.ResponseWriter, r *http.Request, _ error) {
	s.writePageError(w, r, http.StatusInternalServerError, "internal", "the panel request failed")
}

// wantsDocument reports whether this request is a browser navigating to a page.
//
// Sec-Fetch-Dest is the exact answer where it is sent: the browser sets it, script
// cannot, and a fetch or an EventSource never carries "document". Accept is the
// fallback for the clients that do not send it, and it is matched as a whole media
// type - fetch's default `*/*` must not count as asking for a page, or every API
// caller would be handed HTML it cannot parse.
func wantsDocument(r *http.Request) bool {
	if destination := r.Header.Get("Sec-Fetch-Dest"); destination != "" {
		return destination == "document"
	}

	return acceptsHTML(r.Header.Get("Accept"))
}

func acceptsHTML(accept string) bool {
	for _, entry := range strings.Split(accept, ",") {
		media, _, _ := strings.Cut(entry, ";")
		if strings.TrimSpace(media) == "text/html" {
			return true
		}
	}

	return false
}

// writeErrorDocument serves the panel's own page at the status the request earned.
//
// The status stays on the response - a page that says 404 while the wire says 200
// is a lie to every cache and crawler between here and the reader - and the body is
// the same bundle the panel always boots, told which error to render.
func (s *Server) writeErrorDocument(w http.ResponseWriter, status int, code, message string) {
	descriptor, err := json.Marshal(map[string]any{
		"status": status, "code": code, "message": message,
	})
	if err != nil {
		writeError(w, status, code, message)

		return
	}
	// Both substitutions are escaped before they go in, and each lands in a place
	// that escaping covers: the descriptor inside a double-quoted attribute value,
	// the noscript line as element content. Everything around them is the embedded
	// bundle. Callers pass literals today, and this holds even when one stops.
	page := strings.NewReplacer(
		errorSentinel, html.EscapeString(string(descriptor)),
		noscriptSentinel, html.EscapeString(strconv.Itoa(status)+" - "+message),
	).Replace(s.assets.errorPage)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Never stored: the status belongs to this request, and a cached 404 would
	// outlive the invitation or session that made it one.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(page)) //nolint:gosec // Escaped above; the rest is the embedded bundle.
}
