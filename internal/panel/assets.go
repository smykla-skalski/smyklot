package panel

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
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

// The two documents the panel serves by name rather than as plain static files:
// the shell every navigable route resolves to, and the worker, which is read and
// rewritten rather than passed through.
const (
	indexAsset         = "index.html"
	serviceWorkerAsset = "sw.js"
)

type assetBundle struct {
	files     fs.FS
	index     []byte
	indexETag string
	// The same document with the error placeholders still in it, so an error
	// response can fill them per request. Kept apart from index rather than
	// re-derived: index is served on the hot path and must not carry them.
	errorPage     string
	serviceWorker []byte
}

func newAssetBundle(cfg Config) (*assetBundle, error) {
	index, err := fs.ReadFile(cfg.Assets, indexAsset)
	if err != nil {
		return nil, fmt.Errorf("read panel index: %w", err)
	}
	rewritten := strings.ReplaceAll(string(index), basePathSentinel, cfg.BasePath)
	rewritten = strings.ReplaceAll(rewritten, versionSentinel, html.EscapeString(cfg.Version))
	rewritten = strings.ReplaceAll(rewritten, serviceSentinel, html.EscapeString(cfg.ServiceHost))
	// An error document is built by substitution, so a missing placeholder would
	// silently serve a page that reports nothing. Fail at startup instead.
	for _, sentinel := range []string{errorSentinel, noscriptSentinel} {
		if strings.Count(rewritten, sentinel) != 1 {
			return nil, fmt.Errorf("panel index must carry %s exactly once", sentinel)
		}
	}
	served := strings.NewReplacer(errorSentinel, "", noscriptSentinel, defaultNoscript).
		Replace(rewritten)
	serviceWorker, err := fs.ReadFile(cfg.Assets, serviceWorkerAsset)
	if err != nil {
		return nil, fmt.Errorf("read panel service worker: %w", err)
	}
	encodedVersion, err := json.Marshal(cfg.Version)
	if err != nil {
		return nil, fmt.Errorf("encode panel version: %w", err)
	}
	rewrittenWorker := string(serviceWorker)
	for _, placeholder := range []string{`"` + versionSentinel + `"`, `'` + versionSentinel + `'`} {
		rewrittenWorker = strings.ReplaceAll(rewrittenWorker, placeholder, string(encodedVersion))
	}
	if strings.Contains(rewrittenWorker, versionSentinel) {
		return nil, fmt.Errorf("rewrite panel service worker version")
	}

	return &assetBundle{
		files:         cfg.Assets,
		index:         []byte(served),
		indexETag:     fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(served))),
		errorPage:     rewritten,
		serviceWorker: []byte(rewrittenWorker),
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
	content, err := fs.ReadFile(s.assets.files, relative)
	if err != nil {
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
		content = s.assets.serviceWorker
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(relative, "assets/") {
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
