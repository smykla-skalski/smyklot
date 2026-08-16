package panel

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"path"
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
	serviceWorkerAsset = "sw.js"
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
		rewritten := strings.ReplaceAll(string(content), basePathSentinel, cfg.BasePath)
		rewritten = strings.ReplaceAll(rewritten, versionSentinel, html.EscapeString(cfg.Version))
		rewritten = strings.ReplaceAll(rewritten, serviceSentinel, html.EscapeString(cfg.ServiceHost))
		files[p] = []byte(rewritten)
		if p == indexAsset {
			indexRaw = rewritten
		}
		return nil
	})
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
