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
	basePathSentinel           = "/__smyklot_panel_base__"
	versionSentinel            = "__smyklot_panel_version__"
	serviceSentinel            = "__smyklot_panel_service__"
	panelHistoryPath           = "history"
	panelHistoryAuditPath      = "audit"
	panelHistoryFailuresPath   = "failures"
	panelInboxPath             = "inbox"
	panelInvitationsPath       = "invitations"
	panelInstallationsResource = "installations"
	panelRepositoriesPath      = "repositories"
	panelSettingsPath          = "settings"
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
		if isPanelNavigationPath(relative) {
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

func isPanelNavigationPath(relative string) bool {
	parts := strings.Split(strings.Trim(relative, "/"), "/")
	if isRootNavigationPath(parts) {
		return true
	}
	// A page of the reader's own is the whole address; nothing hangs off it.
	if len(parts) == 1 && parts[0] == panelInboxPath {
		return true
	}
	if len(parts) == 2 && parts[0] == "invite" && validInvitationToken(parts[1]) {
		return true
	}
	if len(parts) < 3 || parts[0] != "i" || parts[1] == "" {
		return false
	}

	return isPanelViewPath(parts[2], parts[3:])
}

// A view, and whatever the panel writes after it: history's table, or the
// segments of a dialog standing on the view.
//
// The dialog segments are counted rather than read. Their grammar lives in the
// frontend's route-dialogs, where a name is a repository somebody chose or a
// login somebody registered, and a second copy of it here is a copy that drifts
// - which is exactly what happened: every dialog address the panel writes was
// refused by this function, so a link to one, or a reload of one, answered with
// the not-found page. What is still checked is the part that is ours to know:
// the view has to be a view, and a dialog is one segment or two.
func isPanelViewPath(view string, trailing []string) bool {
	switch view {
	case panelSettingsPath:
		return len(trailing) == 0
	case panelHistoryPath:
		return len(trailing) == 0 || (len(trailing) == 1 && isPanelHistorySection(trailing[0]))
	case panelRepositoriesPath, panelUsersResource, panelInvitationsPath:
		return isDialogSegments(trailing)
	default:
		return false
	}
}

func isDialogSegments(trailing []string) bool {
	if len(trailing) > 2 {
		return false
	}
	for _, segment := range trailing {
		if segment == "" {
			return false
		}
	}

	return true
}

func isRootNavigationPath(parts []string) bool {
	if parts[0] != "root" {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	switch parts[1] {
	case panelSettingsPath:
		return len(parts) == 2
	case panelHistoryPath:
		return len(parts) == 2 || (len(parts) == 3 && isPanelHistorySection(parts[2]))
	case "access":
		if len(parts) == 2 {
			return true
		}
		if parts[2] != panelUsersResource && parts[2] != panelInvitationsPath {
			return false
		}

		// The console's tables take the same dialog grammar as an installation's.
		return isDialogSegments(parts[3:])
	case panelInstallationsResource:
		if len(parts) == 2 {
			return true
		}
		if len(parts) < 4 || parts[2] == "" {
			return false
		}

		return isPanelViewPath(parts[3], parts[4:])
	default:
		return false
	}
}

func isPanelHistorySection(value string) bool {
	return value == panelHistoryAuditPath || value == panelHistoryFailuresPath
}

func validInvitationToken(token string) bool {
	return len(token) == 43 && !strings.ContainsFunc(token, func(r rune) bool {
		return r != '-' && r != '_' && (r < '0' || r > '9') &&
			(r < 'A' || r > 'Z') && (r < 'a' || r > 'z')
	})
}

func (s *Server) writeIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", s.assets.indexETag)
	http.ServeContent(w, r, indexAsset, time.Time{}, bytes.NewReader(s.assets.index))
}
