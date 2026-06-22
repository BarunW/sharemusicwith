package api

import (
	"encoding/json"
	"net/http"

	"connect-with-playlist-wasm/internal/handle"
)

type eventRequest struct {
	Handle    string `json:"handle"`
	EventType string `json:"eventType"`
	LinkID    string `json:"linkId"`
	Platform  string `json:"platform"`
}

var allowedEventTypes = map[string]bool{
	"page_view": true,
	"link_open": true,
	"link_play": true,
}

// POST /api/events — fire-and-forget click/view metrics. Always 202 on a
// well-formed request; referrer/user-agent are read server-side, not trusted
// from the body.
func (s *Server) recordEvent(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEventBytes)
	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "")
		return
	}
	if !allowedEventTypes[req.EventType] {
		writeError(w, http.StatusUnprocessableEntity, "invalid_event", "")
		return
	}
	h := handle.Normalize(req.Handle)
	if h == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_handle", "")
		return
	}

	// Derive anti-gaming signals server-side: a salted visitor hash (for daily
	// dedup), a salted IP hash, and a bot flag. The raw IP is never stored.
	ua := r.Header.Get("User-Agent")
	ip := clientIP(r, s.trustedProxies)
	vh := visitorHash(s.metricsSalt, ip, ua)
	ih := ipHashOf(s.metricsSalt, ip)
	bot := isBotUA(ua)

	// Unknown handles insert nothing; ignore the error so a stray beacon never
	// surfaces as a client-visible failure.
	_ = s.store.InsertEvent(r.Context(), h, req.EventType, req.LinkID, req.Platform,
		r.Header.Get("Referer"), ua, vh, ih, bot)

	w.WriteHeader(http.StatusAccepted)
}
