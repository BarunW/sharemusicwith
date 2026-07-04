package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOriginOnly(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	gate := sameOriginOnly(inner)

	tests := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"no headers (curl, address bar)", nil, 200},
		{"same-origin fetch", map[string]string{"Sec-Fetch-Site": "same-origin"}, 200},
		{"direct navigation", map[string]string{"Sec-Fetch-Site": "none"}, 200},
		{"cross-site fetch header", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"foreign origin", map[string]string{"Origin": "https://evil.example"}, 403},
		{"own origin", map[string]string{"Origin": "https://sharemusicwith.live"}, 200},
		{"own origin, other port", map[string]string{"Origin": "https://sharemusicwith.live:8443"}, 200},
		{"foreign referer", map[string]string{"Referer": "https://evil.example/page"}, 403},
		{"own referer", map[string]string{"Referer": "https://sharemusicwith.live/@someone"}, 200},
		{"garbage origin", map[string]string{"Origin": "not a url"}, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/youtube/playlists/x/tracks", nil)
			r.Host = "sharemusicwith.live"
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			gate.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}
