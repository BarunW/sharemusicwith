package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"connect-with-playlist-wasm/internal/youtube"
)

// The endpoint is public (anonymous visitors must be able to call it), so the
// YouTube quota is defended in layers instead of auth: the per-IP rate
// limiter, a cache that also remembers not-found IDs (so spraying random IDs
// stops costing quota after the first miss), and a global daily budget of
// upstream fetches that bounds worst-case burn no matter how many IPs attack.
const (
	ytCacheTTL        = 15 * time.Minute
	ytCacheMaxEntries = 1024
)

// ytCacheEntry with a nil playlist records a known not-found ID.
type ytCacheEntry struct {
	fetchedAt time.Time
	playlist  *youtube.Playlist
}

type ytCache struct {
	mu      sync.Mutex
	entries map[string]ytCacheEntry
}

func (c *ytCache) get(id string) (*youtube.Playlist, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	if !ok || time.Since(e.fetchedAt) > ytCacheTTL {
		return nil, false
	}
	return e.playlist, true
}

func (c *ytCache) put(id string, p *youtube.Playlist) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]ytCacheEntry)
	}
	if len(c.entries) >= ytCacheMaxEntries {
		for k, e := range c.entries {
			if time.Since(e.fetchedAt) > ytCacheTTL {
				delete(c.entries, k)
			}
		}
		// Still full of live entries: drop an arbitrary one rather than grow.
		if len(c.entries) >= ytCacheMaxEntries {
			for k := range c.entries {
				delete(c.entries, k)
				break
			}
		}
	}
	c.entries[id] = ytCacheEntry{fetchedAt: time.Now(), playlist: p}
}

// ytBudget counts upstream fetches per UTC day. Google's quota resets at
// midnight Pacific; a UTC boundary is close enough for a safety cap.
type ytBudget struct {
	mu   sync.Mutex
	day  int
	used int
}

// allow consumes one fetch from the daily budget; max <= 0 disables the cap.
func (b *ytBudget) allow(max int) bool {
	if max <= 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UTC()
	day := now.Year()*1000 + now.YearDay()
	if day != b.day {
		b.day, b.used = day, 0
	}
	if b.used >= max {
		return false
	}
	b.used++
	return true
}

// GET /api/youtube/playlists/{playlistID}/tracks
func (s *Server) getYouTubeTracks(w http.ResponseWriter, r *http.Request) {
	if s.yt == nil {
		writeError(w, http.StatusServiceUnavailable, "youtube_not_configured",
			"Track listing is not enabled on this server (YOUTUBE_API_KEY is unset).")
		return
	}

	id, err := youtube.NormalizeID(r.PathValue("playlistID"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_playlist_id",
			"That does not look like a YouTube playlist ID.")
		return
	}

	if p, ok := s.ytTracks.get(id); ok {
		if p == nil {
			writeNoSuchPlaylist(w)
			return
		}
		setPublicCache(w, 300)
		writeJSON(w, http.StatusOK, p)
		return
	}

	if !s.ytBudget.allow(s.ytBudgetMax) {
		writeError(w, http.StatusServiceUnavailable, "youtube_quota",
			"YouTube lookups are temporarily rate-limited. Try again later.")
		return
	}

	// Paging a long playlist takes several upstream round-trips; give it a
	// bounded window independent of the client hanging up early.
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	p, err := s.yt.FetchPlaylist(ctx, id)
	switch {
	case errors.Is(err, youtube.ErrNotFound):
		s.ytTracks.put(id, nil)
		writeNoSuchPlaylist(w)
		return
	case errors.Is(err, youtube.ErrQuota):
		writeError(w, http.StatusServiceUnavailable, "youtube_quota",
			"YouTube lookups are temporarily rate-limited. Try again later.")
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, "youtube_error", "Could not fetch the playlist from YouTube.")
		return
	}

	s.ytTracks.put(id, p)
	setPublicCache(w, 300)
	writeJSON(w, http.StatusOK, p)
}

func writeNoSuchPlaylist(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "playlist_not_found",
		"No public playlist with that ID (mixes and private playlists cannot be listed).")
}
