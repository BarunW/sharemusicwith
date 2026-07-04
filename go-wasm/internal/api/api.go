// Package api wires the HTTP routes for the playlist page server: a small JSON
// API plus static asset serving with an SPA fallback for @handle deep links.
package api

import (
	"net"
	"net/http"
	"path/filepath"

	"connect-with-playlist-wasm/internal/config"
	"connect-with-playlist-wasm/internal/store"
	"connect-with-playlist-wasm/internal/youtube"
)

// Server holds handler dependencies.
type Server struct {
	store        *store.Store
	staticDir    string
	staticDirAbs string

	metricsSalt    string
	trustedProxies []*net.IPNet

	yt          *youtube.Client // nil when YOUTUBE_API_KEY is unset
	ytTracks    ytCache
	ytBudget    ytBudget
	ytBudgetMax int
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
	if cfg.YouTubeAPIKey != "" {
		s.yt = youtube.New(cfg.YouTubeAPIKey)
		s.ytBudgetMax = cfg.YouTubeDailyBudget
	}

	// rl throttles unauthenticated reads/beacons; writeRL is a tighter bucket
	// for writes (create/update), since create is unauthenticated and a loop
	// could otherwise mint unlimited rows.
	rl := newRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst, cfg.TrustedProxies)
	writeRL := newRateLimiter(cfg.RateLimitWriteRPS, cfg.RateLimitWriteBurst, cfg.TrustedProxies)

	mux := http.NewServeMux()

	// JSON API. Every endpoint is rate-limited per client IP except /healthz
	// (probed by nginx/monitoring) and static assets (served by nginx in prod).
	mux.Handle("POST /api/playlists", writeRL.middleware(http.HandlerFunc(s.createPlaylist)))
	mux.Handle("GET /api/playlists/{handle}", rl.middleware(http.HandlerFunc(s.getPublicPlaylist)))
	mux.Handle("GET /api/playlists/{handle}/edit/{editToken}", rl.middleware(http.HandlerFunc(s.getEditPlaylist)))
	mux.Handle("PUT /api/playlists/{handle}/edit/{editToken}", writeRL.middleware(http.HandlerFunc(s.updatePlaylist)))
	mux.Handle("GET /api/handles/{handle}/available", rl.middleware(http.HandlerFunc(s.checkHandleAvailable)))
	mux.Handle("POST /api/events", rl.middleware(http.HandlerFunc(s.recordEvent)))
	mux.Handle("GET /api/discover", rl.middleware(http.HandlerFunc(s.discover)))
	// Same-origin gate first: foreign-site browser calls are refused before
	// they consume rate tokens or YouTube quota.
	mux.Handle("GET /api/youtube/playlists/{playlistID}/tracks", sameOriginOnly(rl.middleware(http.HandlerFunc(s.getYouTubeTracks))))
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
