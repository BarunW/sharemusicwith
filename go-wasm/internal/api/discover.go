package api

import (
	"net/http"
	"strconv"
)

// discoverItem is one card in a discovery feed. It carries the deduped counts so
// the client can render a section-appropriate badge without trusting any
// client-side total.
type discoverItem struct {
	Handle       string `json:"handle"`
	Title        string `json:"title"`
	DisplayName  string `json:"displayName"`
	Platform     string `json:"platform,omitempty"`
	LinkCount    int    `json:"linkCount"`
	UniqueViews  int64  `json:"uniqueViews"`
	EngagedPlays int64  `json:"engagedPlays"`
	Views24h     int64  `json:"views24h"`
	Plays24h     int64  `json:"plays24h"`
}

type discoverResponse struct {
	Section string         `json:"section"`
	Items   []discoverItem `json:"items"`
}

var discoverSections = map[string]bool{
	"trending": true,
	"today":    true,
	"popular":  true,
	"new":      true,
}

// GET /api/discover?section=trending|today|popular|new&limit=N — ranked feed read
// from the precomputed playlist_rankings table. Public + cacheable.
func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	section := r.URL.Query().Get("section")
	if !discoverSections[section] {
		section = "trending"
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	rows, err := s.store.ListRankings(r.Context(), section, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "discover_failed", "")
		return
	}
	out := discoverResponse{Section: section, Items: make([]discoverItem, 0, len(rows))}
	for _, it := range rows {
		out.Items = append(out.Items, discoverItem{
			Handle:       it.Handle,
			Title:        it.Title,
			DisplayName:  it.DisplayName,
			Platform:     it.Platform,
			LinkCount:    it.LinkCount,
			UniqueViews:  it.UniqueViews,
			EngagedPlays: it.EngagedPlays,
			Views24h:     it.Views24h,
			Plays24h:     it.Plays24h,
		})
	}
	setPublicCache(w, 30)
	writeJSON(w, http.StatusOK, out)
}
