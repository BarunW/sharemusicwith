package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeAPI serves a two-page playlist of n tracks (plus one deleted entry on
// the first page) and records how many requests it saw.
func fakeAPI(t *testing.T, n int) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/playlists", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("id") != "PLtest" {
			fmt.Fprint(w, `{"items":[]}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"snippet":{"title":"Road Trip","channelTitle":"Barun"}}]}`)
	})
	mux.HandleFunc("/playlistItems", func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("playlistId") != "PLtest" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":{"errors":[{"reason":"playlistNotFound"}]}}`)
			return
		}
		start, next := 0, `"nextPageToken":"p2",`
		if q.Get("pageToken") == "p2" {
			start, next = 50, ""
		}
		items := ""
		for i := start; i < start+50 && i < n; i++ {
			if items != "" {
				items += ","
			}
			items += fmt.Sprintf(`{"snippet":{"title":"Song %d","videoOwnerChannelTitle":"Artist %d - Topic","position":%d,"resourceId":{"videoId":"vid%d"},"thumbnails":{"medium":{"url":"https://i.ytimg.com/%d.jpg"}}}}`, i, i, i, i, i)
		}
		if start == 0 {
			// A deleted video: no thumbnails, no owner — must be skipped.
			items += `,{"snippet":{"title":"Deleted video","position":99,"resourceId":{"videoId":"gone"},"thumbnails":{}}}`
		}
		if n <= 50 {
			next = ""
		}
		fmt.Fprintf(w, `{%s"pageInfo":{"totalResults":%d},"items":[%s]}`, next, n, items)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &calls
}

func testClient(srv *httptest.Server) *Client {
	c := New("test-key")
	c.BaseURL = srv.URL
	return c
}

func TestFetchPlaylistPagesAndSkipsDeleted(t *testing.T) {
	srv, _ := fakeAPI(t, 80)
	pl, err := testClient(srv).FetchPlaylist(context.Background(), "PLtest")
	if err != nil {
		t.Fatal(err)
	}
	if pl.Title != "Road Trip" || pl.Channel != "Barun" {
		t.Errorf("meta = %q/%q, want Road Trip/Barun", pl.Title, pl.Channel)
	}
	if len(pl.Tracks) != 80 {
		t.Fatalf("got %d tracks, want 80 (deleted entry must be skipped)", len(pl.Tracks))
	}
	if pl.Total != 80 || pl.Truncated {
		t.Errorf("total=%d truncated=%v, want 80/false", pl.Total, pl.Truncated)
	}
	tr := pl.Tracks[0]
	if tr.VideoID != "vid0" || tr.Title != "Song 0" || tr.Artist != "Artist 0" || tr.Thumbnail == "" {
		t.Errorf("first track = %+v (artist must have ' - Topic' stripped)", tr)
	}
	if pl.Tracks[79].VideoID != "vid79" {
		t.Errorf("last track = %+v, want vid79", pl.Tracks[79])
	}
}

func TestFetchPlaylistTruncatesAtMaxTracks(t *testing.T) {
	srv, calls := fakeAPI(t, 80)
	c := testClient(srv)
	c.MaxTracks = 50
	pl, err := c.FetchPlaylist(context.Background(), "PLtest")
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Tracks) != 50 || !pl.Truncated {
		t.Fatalf("got %d tracks truncated=%v, want 50/true", len(pl.Tracks), pl.Truncated)
	}
	// meta + first items page only — the second page must not be fetched.
	if *calls != 2 {
		t.Errorf("api calls = %d, want 2", *calls)
	}
}

func TestFetchPlaylistNotFound(t *testing.T) {
	srv, _ := fakeAPI(t, 10)
	_, err := testClient(srv).FetchPlaylist(context.Background(), "PLmissing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFetchPlaylistQuotaExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"error":{"errors":[{"reason":"quotaExceeded"}]}}`)
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(srv).FetchPlaylist(context.Background(), "PLtest")
	if !errors.Is(err, ErrQuota) {
		t.Fatalf("err = %v, want ErrQuota", err)
	}
}

func TestNormalizeID(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"PLMC9KNkIncKtPzgY-5rmhvj7fax8fdxoj", "PLMC9KNkIncKtPzgY-5rmhvj7fax8fdxoj", true},
		{" PLabc123 ", "PLabc123", true},
		{"VLPLabc123", "PLabc123", true},                    // music.youtube.com browse-ID wrapper
		{"OLAK5uy_kkl6WOHzhBtwd6uwynLBpSC9RTMCEbwlM", "OLAK5uy_kkl6WOHzhBtwd6uwynLBpSC9RTMCEbwlM", true}, // album
		{"", "", false},
		{"PL abc", "", false},
		{"PL<script>", "", false},
	}
	for _, tt := range tests {
		got, err := NormalizeID(tt.in)
		if tt.ok && (err != nil || got != tt.want) {
			t.Errorf("NormalizeID(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
		if !tt.ok && err == nil {
			t.Errorf("NormalizeID(%q) = %q, want error", tt.in, got)
		}
	}
}
