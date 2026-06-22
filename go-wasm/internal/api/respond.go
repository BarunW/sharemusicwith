package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

const (
	maxBodyBytes  = 64 << 10 // playlist create/update
	maxEventBytes = 4 << 10  // metric events
)

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorResponse{Error: code, Message: msg})
}

func setNoCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// setPublicCache marks a response cacheable by browsers/CDN for maxAge seconds,
// serving stale up to 60s while revalidating. Used by the public discovery feed.
func setPublicCache(w http.ResponseWriter, maxAge int) {
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge)+", stale-while-revalidate=60")
}
