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

// setRevalidateCache lets browsers and the CDN store the response but check with
// the origin on every use (cheap 304s via Last-Modified). Used for the HTML
// shell and assets whose URL is stable across deploys (styles.css, wasm_exec.js)
// so a fresh deploy is always picked up.
func setRevalidateCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache")
}

// setImmutableCache marks content-hashed assets (e.g. main.<hash>.wasm) as
// cacheable for a year and never revalidated — the filename changes whenever the
// bytes do, so the CDN can edge-cache them and browsers never re-download an
// unchanged build.
func setImmutableCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
}

// setPublicCache marks a response cacheable by browsers/CDN for maxAge seconds,
// serving stale up to 60s while revalidating. Used by the public discovery feed.
func setPublicCache(w http.ResponseWriter, maxAge int) {
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge)+", stale-while-revalidate=60")
}
