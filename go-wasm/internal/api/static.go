package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// assetExts are file types that must 404 when missing rather than fall back to
// index.html — otherwise a broken/stale build would silently serve HTML for a
// .wasm request and the loader would report "Go WASM failed to load".
var assetExts = map[string]bool{
	".wasm": true, ".js": true, ".css": true, ".map": true,
	".png": true, ".jpg": true, ".jpeg": true, ".svg": true,
	".gif": true, ".webp": true, ".ico": true, ".json": true,
}

// serveStaticOrIndex serves real files from staticDir and falls back to the SPA
// shell (index.html) for client routes ("/", "/created", "/@handle...").
func (s *Server) serveStaticOrIndex(w http.ResponseWriter, r *http.Request) {
	setNoCache(w)
	upath := r.URL.Path

	if upath == "/" || upath == "/created" || strings.HasPrefix(upath, "/@") {
		s.serveIndex(w, r)
		return
	}

	clean := filepath.Clean(upath)
	full := filepath.Join(s.staticDirAbs, clean)
	// Reject anything that escapes the static root.
	if full != s.staticDirAbs && !strings.HasPrefix(full, s.staticDirAbs+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(full); err == nil && !info.IsDir() {
		http.ServeFile(w, r, full)
		return
	}
	if assetExts[strings.ToLower(filepath.Ext(upath))] {
		http.NotFound(w, r)
		return
	}
	s.serveIndex(w, r)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(s.staticDirAbs, "index.html"))
}
