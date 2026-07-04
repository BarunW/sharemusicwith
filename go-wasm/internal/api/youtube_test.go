package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"connect-with-playlist-wasm/internal/youtube"
)

func ytRequest(t *testing.T, s *Server, id string) *httptest.ResponseRecorder {
	t.Helper()
	// The handler only reads the path value, so the request URL can stay fixed
	// (NewRequest panics on IDs that aren't URL-safe, which is a case we test).
	r := httptest.NewRequest(http.MethodGet, "/api/youtube/playlists/x/tracks", nil)
	r.SetPathValue("playlistID", id)
	w := httptest.NewRecorder()
	s.getYouTubeTracks(w, r)
	return w
}

func TestYouTubeTracksNotConfigured(t *testing.T) {
	w := ytRequest(t, &Server{}, "PLabc123")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestYouTubeTracksInvalidID(t *testing.T) {
	s := &Server{yt: youtube.New("k")}
	w := ytRequest(t, s, "not a playlist id")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestYouTubeTracksFetchAndCache(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		switch r.URL.Path {
		case "/playlists":
			fmt.Fprint(w, `{"items":[{"snippet":{"title":"Mix","channelTitle":"Barun"}}]}`)
		case "/playlistItems":
			fmt.Fprint(w, `{"pageInfo":{"totalResults":1},"items":[{"snippet":{"title":"Song","videoOwnerChannelTitle":"Artist - Topic","position":0,"resourceId":{"videoId":"v1"},"thumbnails":{"default":{"url":"https://i.ytimg.com/v1.jpg"}}}}]}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(upstream.Close)

	yt := youtube.New("k")
	yt.BaseURL = upstream.URL
	s := &Server{yt: yt}

	w := ytRequest(t, s, "PLabc123")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	var pl youtube.Playlist
	if err := json.Unmarshal(w.Body.Bytes(), &pl); err != nil {
		t.Fatal(err)
	}
	if pl.Title != "Mix" || len(pl.Tracks) != 1 || pl.Tracks[0].Artist != "Artist" {
		t.Fatalf("unexpected playlist: %+v", pl)
	}
	if got := upstreamCalls; got != 2 { // playlists + playlistItems
		t.Fatalf("upstream calls = %d, want 2", got)
	}

	// Second request must be served from the in-memory cache.
	w = ytRequest(t, s, "PLabc123")
	if w.Code != http.StatusOK {
		t.Fatalf("cached status = %d", w.Code)
	}
	if upstreamCalls != 2 {
		t.Fatalf("upstream calls after cached hit = %d, want still 2", upstreamCalls)
	}
}

func TestYouTubeTracksNotFoundIsNegativeCached(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		fmt.Fprint(w, `{"items":[]}`)
	}))
	t.Cleanup(upstream.Close)

	yt := youtube.New("k")
	yt.BaseURL = upstream.URL
	s := &Server{yt: yt}

	w := ytRequest(t, s, "PLmissing")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	// Spraying the same bad ID again must not reach YouTube.
	w = ytRequest(t, s, "PLmissing")
	if w.Code != http.StatusNotFound {
		t.Fatalf("cached status = %d, want 404", w.Code)
	}
	if upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls)
	}
}

func TestYouTubeTracksDailyBudget(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"items":[]}`)
	}))
	t.Cleanup(upstream.Close)

	yt := youtube.New("k")
	yt.BaseURL = upstream.URL
	s := &Server{yt: yt, ytBudgetMax: 1}

	if w := ytRequest(t, s, "PLfirst"); w.Code != http.StatusNotFound {
		t.Fatalf("first status = %d, want 404", w.Code)
	}
	// Budget spent: a fresh (uncached) ID must be refused, not fetched.
	if w := ytRequest(t, s, "PLsecond"); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("over-budget status = %d, want 503", w.Code)
	}
	// Cached IDs still work with no budget left.
	if w := ytRequest(t, s, "PLfirst"); w.Code != http.StatusNotFound {
		t.Fatalf("cached status = %d, want 404", w.Code)
	}
}
