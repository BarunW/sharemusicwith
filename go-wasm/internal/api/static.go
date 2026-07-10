package api

import (
	"bytes"
	"encoding/xml"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"connect-with-playlist-wasm/internal/handle"
	"connect-with-playlist-wasm/internal/store"
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

// serveStaticOrIndex serves real files and route-aware SPA documents. Public
// profiles are rendered with their stored data so crawlers, unfurlers, and
// browsers all receive the same meaningful initial HTML.
func (s *Server) serveStaticOrIndex(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path

	switch upath {
	case "/":
		s.serveDocument(w, r, http.StatusOK, s.documents.homeDocument())
		return
	case "/create":
		s.serveDocument(w, r, http.StatusOK, s.documents.createDocument())
		return
	case "/created":
		s.serveDocument(w, r, http.StatusOK, s.documents.createdDocument())
		return
	}
	if strings.HasPrefix(upath, "/@") {
		s.serveHandleRoute(w, r)
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
	s.serveDocument(w, r, http.StatusNotFound, s.documents.notFoundDocument())
}

func (s *Server) serveHandleRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 3 && parts[1] == "edit" && parts[2] != "" {
		s.serveDocument(w, r, http.StatusOK, s.documents.editDocument())
		return
	}
	if len(parts) != 1 {
		s.serveDocument(w, r, http.StatusNotFound, s.documents.notFoundDocument())
		return
	}

	rawHandle := strings.TrimPrefix(parts[0], "@")
	h := handle.Normalize(rawHandle)
	if handle.Validate(h) != nil {
		s.serveDocument(w, r, http.StatusNotFound, s.documents.notFoundDocument())
		return
	}
	if rawHandle != h {
		http.Redirect(w, r, "/@"+h, http.StatusPermanentRedirect)
		return
	}

	p, err := s.store.GetByHandle(r.Context(), h)
	if errors.Is(err, store.ErrNotFound) {
		s.serveDocument(w, r, http.StatusNotFound, s.documents.notFoundDocument())
		return
	}
	if err != nil {
		w.Header().Set("Retry-After", "30")
		s.serveDocument(w, r, http.StatusServiceUnavailable, s.documents.unavailableDocument())
		return
	}
	s.serveDocument(w, r, http.StatusOK, s.documents.profileDocument(p))
}

func (s *Server) serveDocument(w http.ResponseWriter, r *http.Request, status int, doc pageDocument) {
	var body bytes.Buffer
	if err := s.documents.template.Execute(&body, doc); err != nil {
		http.Error(w, "Could not render this page.", http.StatusInternalServerError)
		return
	}

	// The shell references content-hashed wasm in production, but the HTML and
	// profile metadata are mutable and must be checked on every navigation.
	setRevalidateCache(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if strings.Contains(doc.Robots, "noindex") {
		w.Header().Set("X-Robots-Tag", doc.Robots)
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body.Bytes())
	}
}

func (s *Server) serveRobots(w http.ResponseWriter, r *http.Request) {
	setPublicCache(w, 3600)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte("User-agent: *\nAllow: /\nDisallow: /created\nDisallow: /*/edit/\nSitemap: " + s.documents.siteURL + "/sitemap.xml\n"))
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Location string `xml:"loc"`
	LastMod  string `xml:"lastmod,omitempty"`
}

func (s *Server) serveSitemap(w http.ResponseWriter, r *http.Request) {
	pages, err := s.store.ListSitemapPages(r.Context())
	if err != nil {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "Could not build sitemap.", http.StatusServiceUnavailable)
		return
	}
	urls := make([]sitemapURL, 0, len(pages)+2)
	urls = append(urls,
		sitemapURL{Location: s.documents.siteURL + "/"},
		sitemapURL{Location: s.documents.siteURL + "/create"},
	)
	for _, page := range pages {
		urls = append(urls, sitemapURL{
			Location: s.documents.siteURL + "/@" + page.Handle,
			LastMod:  page.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	body, err := xml.MarshalIndent(sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}, "", "  ")
	if err != nil {
		http.Error(w, "Could not build sitemap.", http.StatusInternalServerError)
		return
	}
	setPublicCache(w, 600)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(append([]byte(xml.Header), body...))
}
