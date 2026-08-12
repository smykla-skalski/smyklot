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
	panelInvitationsPath       = "invitations"
	panelInstallationsResource = "installations"
	panelRepositoriesPath      = "repositories"
	panelSettingsPath          = "settings"
)

type assetBundle struct {
	files         fs.FS
	index         []byte
	indexETag     string
	serviceWorker []byte
}

func newAssetBundle(cfg Config) (*assetBundle, error) {
	index, err := fs.ReadFile(cfg.Assets, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read panel index: %w", err)
	}
	rewritten := strings.ReplaceAll(string(index), basePathSentinel, cfg.BasePath)
	rewritten = strings.ReplaceAll(rewritten, versionSentinel, html.EscapeString(cfg.Version))
	rewritten = strings.ReplaceAll(rewritten, serviceSentinel, html.EscapeString(cfg.ServiceHost))
	serviceWorker, err := fs.ReadFile(cfg.Assets, "sw.js")
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
		index:         []byte(rewritten),
		indexETag:     fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(rewritten))),
		serviceWorker: []byte(rewrittenWorker),
	}, nil
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	relative := strings.TrimPrefix(r.URL.Path, s.cfg.BasePath)
	relative = strings.TrimPrefix(relative, "/")
	if relative == "" || relative == "index.html" {
		s.writeIndex(w, r)
		return
	}
	if !fs.ValidPath(relative) {
		s.writeError(w, http.StatusNotFound, "not_found", "panel asset not found")
		return
	}
	content, err := fs.ReadFile(s.assets.files, relative)
	if err != nil {
		if isPanelNavigationPath(relative) {
			s.writeIndex(w, r)
		} else {
			s.writeError(w, http.StatusNotFound, "not_found", "panel route not found")
		}
		return
	}
	contentType := mime.TypeByExtension(path.Ext(relative))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if relative == "sw.js" {
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
	trimmed := strings.Trim(relative, "/")
	if isRootNavigationPath(strings.Split(trimmed, "/")) {
		return true
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 2 && parts[0] == "invite" && validInvitationToken(parts[1]) {
		return true
	}
	if (len(parts) != 3 && len(parts) != 4) || parts[0] != "i" || parts[1] == "" {
		return false
	}
	if len(parts) == 4 {
		return parts[2] == panelHistoryPath && isPanelHistorySection(parts[3])
	}

	switch parts[2] {
	case panelSettingsPath, panelRepositoriesPath, panelUsersResource, panelInvitationsPath, panelHistoryPath:
		return true
	default:
		return false
	}
}

func isRootNavigationPath(parts []string) bool {
	if len(parts) == 1 {
		return parts[0] == "root"
	}
	if parts[0] != "root" {
		return false
	}
	if len(parts) == 2 {
		return parts[1] == panelInstallationsResource || parts[1] == "access" ||
			parts[1] == panelHistoryPath || parts[1] == panelSettingsPath
	}
	if len(parts) == 3 {
		return parts[1] == "access" && (parts[2] == panelUsersResource || parts[2] == panelInvitationsPath) ||
			parts[1] == panelHistoryPath &&
				(parts[2] == panelHistoryAuditPath || parts[2] == panelHistoryFailuresPath)
	}
	if (len(parts) != 4 && len(parts) != 5) ||
		parts[1] != panelInstallationsResource || parts[2] == "" {
		return false
	}
	if len(parts) == 5 {
		return parts[3] == panelHistoryPath && isPanelHistorySection(parts[4])
	}
	switch parts[3] {
	case panelSettingsPath, panelRepositoriesPath, panelUsersResource, panelInvitationsPath, panelHistoryPath:
		return true
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
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(s.assets.index))
}
