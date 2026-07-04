package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// sameOriginOnly rejects browser requests initiated by other websites, keeping
// quota-backed endpoints private to this site without needing accounts. It
// layers three signals, strongest first:
//
//   - Sec-Fetch-Site: browsers set it and pages cannot forge it; "cross-site"
//     means another origin's code made the call.
//   - Origin: sent on all cross-origin fetches; must match the request host.
//   - Referer: legacy fallback; checked only when present (privacy settings
//     often strip it).
//
// Non-browser clients (curl, scripts) can omit or fake all three — those are
// bounded by the rate limiter and the upstream daily budget instead. Browsers
// also cannot READ our responses cross-origin (no CORS headers are sent); this
// gate exists so foreign sites cannot even trigger upstream spend.
func sameOriginOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			writeError(w, http.StatusForbidden, "cross_site", "This endpoint only serves this site.")
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "null" && !refersToHost(o, r.Host) {
			writeError(w, http.StatusForbidden, "cross_site", "This endpoint only serves this site.")
			return
		}
		if ref := r.Header.Get("Referer"); ref != "" && !refersToHost(ref, r.Host) {
			writeError(w, http.StatusForbidden, "cross_site", "This endpoint only serves this site.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// refersToHost reports whether an Origin/Referer URL points at the same
// hostname the request was addressed to (ports ignored: TLS terminates at
// nginx, so internal and external ports differ legitimately).
func refersToHost(rawURL, requestHost string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := requestHost
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		host = h
	}
	return strings.EqualFold(u.Hostname(), host)
}
