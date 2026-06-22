package api

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

// fingerprinted matches assets whose filename embeds a content hash
// (e.g. main.1a2b3c4d5e6f.wasm) — the bytes can't change without the URL
// changing, so they're safe to cache forever. The Docker build hashes main.wasm
// and rewrites index.html to match; local dev keeps the plain name (revalidated).
var fingerprinted = regexp.MustCompile(`\.[0-9a-f]{8,}\.(wasm|js|css)$`)

// serveStaticOrIndex serves real files from staticDir and falls back to the SPA
// shell (index.html) for client routes ("/", "/created", "/@handle...").
func (s *Server) serveStaticOrIndex(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path

	if upath == "/" || upath == "/created" || strings.HasPrefix(upath, "/@") {
		s.serveIndex(w, r)
		return
	}

	// Standalone static legal page (no SPA/wasm). Served from a clean URL; the
	// footer links here. Kept out of the SPA so it loads instantly without the
	// wasm bundle.
	if upath == "/privacy" {
		setRevalidateCache(w)
		http.ServeFile(w, r, filepath.Join(s.staticDirAbs, "privacy.html"))
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
		// Content-hashed assets (main.<hash>.wasm) never change under a given
		// URL, so cache them forever; everything else must revalidate so a new
		// deploy is picked up.
		if fingerprinted.MatchString(clean) {
			setImmutableCache(w)
		} else {
			setRevalidateCache(w)
		}
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
	// The shell references the content-hashed wasm by URL, so it must always be
	// revalidated — otherwise a returning visitor could load an old shell that
	// points at a wasm file the new deploy no longer has.
	setRevalidateCache(w)
	http.ServeFile(w, r, filepath.Join(s.staticDirAbs, "index.html"))
}
