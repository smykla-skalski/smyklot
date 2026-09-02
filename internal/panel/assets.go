package panel

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	basePathSentinel         = "/__smyklot_panel_base__"
	versionSentinel          = "__smyklot_panel_version__"
	serviceSentinel          = "__smyklot_panel_service__"
	panelHistoryAuditPath    = "audit"
	panelHistoryFailuresPath = "failures"
	panelInvitationsPath     = "invitations"
	panelWorkspacesResource  = "workspaces"
)

// The documents the panel serves by name rather than as plain static files.
const (
	indexAsset         = "index.html"
	serviceWorkerAsset = "service-worker.js"
)

// Text file extensions whose sentinels the server rewrites at startup. Binary
// files (images, fonts) are stored verbatim.
var textExtensions = map[string]bool{
	".html": true,
	".js":   true,
	".mjs":  true,
	".css":  true,
	".json": true,
	".svg":  true,
	".map":  true,
}

var scriptSourceAttribute = regexp.MustCompile(`(?i)(?:^|\s)src\s*=`)

type assetBundle struct {
	files     map[string][]byte
	index     []byte
	indexETag string
	// The addresses the panel's router has a page for. Generated into the bundle
	// from `src/routes`; see routes.go.
	routes *routeTable
	// The index with the error and noscript placeholders still in it, so an
	// error response can fill them per request. Kept apart from index rather
	// than re-derived: index is served on the hot path and must not carry them.
	errorPage string
}

func newAssetBundle(cfg Config) (*assetBundle, error) {
	files, indexRaw, err := loadAssetFiles(cfg)
	if err != nil {
		return nil, fmt.Errorf("walk panel assets: %w", err)
	}

	if indexRaw == "" {
		return nil, fmt.Errorf("panel bundle has no %s", indexAsset)
	}

	// An error document is built by substitution, so a missing placeholder would
	// silently serve a page that reports nothing. Fail at startup instead.
	for _, sentinel := range []string{errorSentinel, noscriptSentinel} {
		if strings.Count(indexRaw, sentinel) != 1 {
			return nil, fmt.Errorf("panel index must carry %s exactly once", sentinel)
		}
	}
	served := strings.NewReplacer(errorSentinel, "", noscriptSentinel, defaultNoscript).
		Replace(indexRaw)

	// The route table is read out of the bundle, never served from it. A build that
	// did not write one leaves the server unable to tell a panel address from a
	// typing mistake, so refuse to start rather than guess at either.
	manifest, ok := files[routeManifestAsset]
	if !ok {
		return nil, fmt.Errorf("panel bundle has no %s", routeManifestAsset)
	}
	routes, err := loadRouteTable(manifest)
	if err != nil {
		return nil, err
	}
	delete(files, routeManifestAsset)

	// index.html is served via writeIndex, not from the file map.
	delete(files, indexAsset)

	// Fail-closed: no served text asset may retain an unresolved sentinel.
	for p, content := range files {
		if strings.Contains(string(content), basePathSentinel) ||
			strings.Contains(string(content), versionSentinel) ||
			strings.Contains(string(content), serviceSentinel) {
			return nil, fmt.Errorf("panel asset %s retains an unresolved sentinel after rewrite", p)
		}
	}

	return &assetBundle{
		files:     files,
		index:     []byte(served),
		indexETag: fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(served))),
		routes:    routes,
		errorPage: indexRaw,
	}, nil
}

func loadAssetFiles(cfg Config) (map[string][]byte, string, error) {
	files := make(map[string][]byte)
	var indexRaw string

	err := fs.WalkDir(cfg.Assets, ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(cfg.Assets, p)
		if err != nil {
			return err
		}
		if !textExtensions[path.Ext(p)] {
			files[p] = content
			return nil
		}
		rewritten := rewriteAssetText(p, string(content), cfg)
		if p == indexAsset {
			rewritten, err = refreshInlineScriptHashes(string(content), rewritten)
			if err != nil {
				return fmt.Errorf("refresh panel CSP hashes: %w", err)
			}
		}
		files[p] = []byte(rewritten)
		if p == indexAsset {
			indexRaw = rewritten
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	return files, indexRaw, nil
}

func rewriteAssetText(assetPath, content string, cfg Config) string {
	if assetPath == indexAsset {
		content = strings.ReplaceAll(content, basePathSentinel, html.EscapeString(cfg.BasePath))
		content = strings.ReplaceAll(content, versionSentinel, html.EscapeString(cfg.Version))

		return strings.ReplaceAll(content, serviceSentinel, html.EscapeString(cfg.ServiceHost))
	}

	// The generated worker, the version manifest and the bundler's own version
	// export carry these sentinels as complete string literals. Replacing the
	// delimiters too makes one encoding correct for JavaScript and JSON,
	// regardless of which quote style the bundler chose. A naked or embedded
	// sentinel is deliberately left behind so the fail-closed check below
	// rejects a new, context-ambiguous build shape.
	//
	// This reaches every served script, not only the generated ones, and it
	// cannot be narrowed: the chunk holding the bundler's version export is
	// hashed, so there is no name to match on. Panel source must therefore never
	// spell a sentinel - one that did held the version sentinel to recognise an
	// unsubstituted page, got it rewritten into the version itself, and reported
	// every release as having no version at all. `tests/base.test.ts` in the
	// frontend keeps sentinels out of the source this rewrites.
	content = strings.ReplaceAll(content, basePathSentinel, cfg.BasePath)
	content = replaceStringLiteral(content, versionSentinel, cfg.Version)

	return replaceStringLiteral(content, serviceSentinel, cfg.ServiceHost)
}

// SvelteKit hashes its generated inline bootstrap at build time. The bootstrap
// carries the base-path sentinel, so replacing the sentinel changes the script
// bytes and invalidates that hash. Refresh the matching CSP token after every
// HTML rewrite, and fail closed if the generated document no longer has the
// shape the server knows how to secure.
func refreshInlineScriptHashes(original, rewritten string) (string, error) {
	originalScripts, err := inlineScriptBodies(original)
	if err != nil {
		return "", err
	}
	rewrittenScripts, err := inlineScriptBodies(rewritten)
	if err != nil {
		return "", err
	}
	if len(originalScripts) != len(rewrittenScripts) {
		return "", fmt.Errorf(
			"inline script count changed from %d to %d",
			len(originalScripts),
			len(rewrittenScripts),
		)
	}

	for index, originalScript := range originalScripts {
		rewrittenScript := rewrittenScripts[index]
		if originalScript == rewrittenScript {
			continue
		}
		oldHash := quotedScriptHash(originalScript)
		if strings.Count(rewritten, oldHash) != 1 {
			return "", fmt.Errorf("changed inline script %d has no unique CSP hash", index)
		}
		rewritten = strings.Replace(rewritten, oldHash, quotedScriptHash(rewrittenScript), 1)
	}

	return rewritten, nil
}

func inlineScriptBodies(document string) ([]string, error) {
	var bodies []string
	for remaining := document; ; {
		open := strings.Index(remaining, "<script")
		if open == -1 {
			return bodies, nil
		}
		tagEnd := strings.IndexByte(remaining[open:], '>')
		if tagEnd == -1 {
			return nil, fmt.Errorf("inline script opening tag is incomplete")
		}
		tagEnd += open
		bodyStart := tagEnd + 1
		closeOffset := strings.Index(remaining[bodyStart:], "</script>")
		if closeOffset == -1 {
			return nil, fmt.Errorf("script closing tag is missing")
		}
		bodyEnd := bodyStart + closeOffset
		if !scriptSourceAttribute.MatchString(remaining[open:tagEnd]) {
			bodies = append(bodies, remaining[bodyStart:bodyEnd])
		}
		remaining = remaining[bodyEnd+len("</script>"):]
	}
}

func quotedScriptHash(script string) string {
	digest := sha256.Sum256([]byte(script))

	return `'sha256-` + base64.StdEncoding.EncodeToString(digest[:]) + `'`
}

func replaceStringLiteral(content, sentinel, value string) string {
	encoded, _ := json.Marshal(value) // A Go string always has a JSON representation.
	for _, delimiter := range []string{`"`, `'`, "`"} {
		content = strings.ReplaceAll(content, delimiter+sentinel+delimiter, string(encoded))
	}

	return content
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	/* Trimmed at both ends. A trailing slash is not part of the address - the
	   panel's own router reads `/inbox/` as `/inbox` - but `fs.ValidPath` refuses
	   one, so every panel route answered a typed or copied trailing slash with the
	   not-found page. */
	relative := strings.Trim(strings.TrimPrefix(r.URL.Path, s.cfg.BasePath), "/")
	if relative == "" || relative == indexAsset {
		s.writeIndex(w, r)
		return
	}
	if !fs.ValidPath(relative) {
		s.writePageError(w, r, http.StatusNotFound, "not_found", "panel asset not found")
		return
	}
	content, ok := s.assets.files[relative]
	if !ok {
		// The generated patterns are written against an absolute, base-relative path.
		if s.assets.routes.matches("/"+relative) || s.signedOutSeesSignIn(r, relative) {
			s.writeIndex(w, r)
		} else {
			s.writePageError(w, r, http.StatusNotFound, "not_found", "panel route not found")
		}
		return
	}
	contentType := mime.TypeByExtension(path.Ext(relative))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if relative == serviceWorkerAsset {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(relative, "_app/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content) //nolint:gosec // Content comes only from the generated embedded bundle.
}

// signedOutSeesSignIn reports whether an address nothing here serves should be
// answered with the shell anyway, because a reader with no session is shown the
// sign-in card wherever they are and told what went wrong only once there is
// somebody to tell.
//
// This is a RULE ABOUT WHEN ERRORS ARE SHOWN, not a secrecy control, and the
// distinction is worth writing down because it was got wrong here first. The
// original claim was that it hid the panel's route table: a known route shape
// returned the shell, an unknown one the not-found page, and the difference is
// readable by anybody with curl. Closing that difference hides nothing, because
// the route table is in the client bundle. `index.html` is public by necessity -
// the sign-in page is drawn by the same app - and it links the entry chunk,
// which carries every route shape and matcher name as string literals. Two
// requests still enumerate it, and no arrangement of this function changes that
// while the panel is a single-page app.
//
// What it does buy is the reader's experience, which is what it is for: one
// answer to every address until you are signed in, and the not-found page after,
// when it is yours to see.
//
// Names were never the question and stay safe.
// `/workspace/does-not-exist/sync/plan` and a real workspace's plan return the
// same bytes to a stranger, because the route MATCHES either way and which
// workspaces exist is decided behind the API.
//
// PAGE NAVIGATIONS ONLY, on both counts. Anything carrying an extension is a
// file request, and a missing script answered with HTML is a module error rather
// than a 404 - the service worker and the bootstrap both need the plain answer.
// And anything that did not ask for a document keeps the JSON it parses: the
// panel's own fetches reach unregistered API paths when a route is retired or a
// method is wrong, and handing those a page turns a 404 a caller can read into a
// `SyntaxError` it cannot.
func (s *Server) signedOutSeesSignIn(r *http.Request, relative string) bool {
	if path.Ext(relative) != "" || !wantsDocument(r) {
		return false
	}
	_, _, err := s.viewerSession(r)

	return err != nil
}

func (s *Server) writeIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", s.assets.indexETag)
	http.ServeContent(w, r, indexAsset, time.Time{}, bytes.NewReader(s.assets.index))
}
