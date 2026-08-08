package panel

import (
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

const (
	basePathSentinel = "/__smyklot_panel_base__"
	versionSentinel  = "__smyklot_panel_version__"
	serviceSentinel  = "__smyklot_panel_service__"
)

type assetBundle struct {
	files fs.FS
	index []byte
}

func newAssetBundle(cfg Config) (*assetBundle, error) {
	index, err := fs.ReadFile(cfg.Assets, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read panel index: %w", err)
	}
	rewritten := strings.ReplaceAll(string(index), basePathSentinel, cfg.BasePath)
	rewritten = strings.ReplaceAll(rewritten, versionSentinel, html.EscapeString(cfg.Version))
	rewritten = strings.ReplaceAll(rewritten, serviceSentinel, html.EscapeString(cfg.ServiceHost))

	return &assetBundle{files: cfg.Assets, index: []byte(rewritten)}, nil
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	relative := strings.TrimPrefix(r.URL.Path, s.cfg.BasePath)
	relative = strings.TrimPrefix(relative, "/")
	if relative == "" || relative == "index.html" {
		s.writeIndex(w)
		return
	}
	if !fs.ValidPath(relative) {
		s.writeError(w, http.StatusNotFound, "not_found", "panel asset not found")
		return
	}
	content, err := fs.ReadFile(s.assets.files, relative)
	if err != nil {
		s.writeIndex(w)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(relative))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if strings.HasPrefix(relative, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content) //nolint:gosec // Content comes only from the generated embedded bundle.
}

func (s *Server) writeIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.assets.index)
}
