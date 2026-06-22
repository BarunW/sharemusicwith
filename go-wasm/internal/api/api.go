// Package api wires the HTTP routes for the playlist page server: a small JSON
// API plus static asset serving with an SPA fallback for @handle deep links.
package api

import (
	"net"
	"net/http"
	"path/filepath"

	"connect-with-playlist-wasm/internal/config"
	"connect-with-playlist-wasm/internal/store"
)

// Server holds handler dependencies.
type Server struct {
	store        *store.Store
	staticDir    string
	staticDirAbs string

	metricsSalt    string
	trustedProxies []*net.IPNet
}

// NewRouter builds the request multiplexer.
//
// App page routes are distinguished from static assets by the '@' prefix:
// "/@handle" and "/@handle/edit/<token>" are served the SPA shell, while
// everything else is a file lookup. API routes use the bare handle (no '@')
// because Go's ServeMux wildcards must occupy a whole path segment.
func NewRouter(st *store.Store, cfg config.Config) http.Handler {
	abs, err := filepath.Abs(cfg.StaticDir)
	if err != nil {
		abs = cfg.StaticDir
	}
	s := &Server{
		store:          st,
		staticDir:      cfg.StaticDir,
		staticDirAbs:   abs,
		metricsSalt:    cfg.MetricsSalt,
		trustedProxies: cfg.TrustedProxies,
	}

	rl := newRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst, cfg.TrustedProxies)

	mux := http.NewServeMux()

	// JSON API.
	mux.HandleFunc("POST /api/playlists", s.createPlaylist)
	mux.Handle("GET /api/playlists/{handle}", rl.middleware(http.HandlerFunc(s.getPublicPlaylist)))
	mux.HandleFunc("GET /api/playlists/{handle}/edit/{editToken}", s.getEditPlaylist)
	mux.HandleFunc("PUT /api/playlists/{handle}/edit/{editToken}", s.updatePlaylist)
	mux.HandleFunc("GET /api/handles/{handle}/available", s.checkHandleAvailable)
	mux.Handle("POST /api/events", rl.middleware(http.HandlerFunc(s.recordEvent)))
	mux.HandleFunc("GET /api/discover", s.discover)
	mux.HandleFunc("GET /healthz", s.health)

	// Static files + SPA fallback (handles "/", "/created", "/@handle...").
	mux.HandleFunc("/", s.serveStaticOrIndex)

	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "db_down", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
