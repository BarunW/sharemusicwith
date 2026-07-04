package main

import "testing"

const appleMusicSampleURL = "https://music.apple.com/us/playlist/todays-country/pl.87bb5b36a9bd49db8c975607452bfa2b"
const appleMusicSampleEmbedURL = "https://embed.music.apple.com/us/playlist/todays-country/pl.87bb5b36a9bd49db8c975607452bfa2b"

func TestParsePlaylistLinkAppleMusicEmbed(t *testing.T) {
	link, ok := parsePlaylistLink(appleMusicSampleURL)
	if !ok {
		t.Fatal("expected Apple Music URL to parse")
	}
	if link.Platform != "Apple Music" {
		t.Fatalf("platform = %q, want Apple Music", link.Platform)
	}
	if link.Title != "Apple Music playlist" {
		t.Fatalf("title = %q, want Apple Music playlist", link.Title)
	}

	if link.EmbedURL != appleMusicSampleEmbedURL {
		t.Fatalf("embed URL = %q, want %q", link.EmbedURL, appleMusicSampleEmbedURL)
	}
}

func TestMigrateSavedAppleMusicLinkAddsEmbed(t *testing.T) {
	state := State{
		Playlist: Playlist{
			Links: []Link{
				{
					ID:       "saved-apple",
					Name:     "Apple Music link",
					URL:      appleMusicSampleURL,
					Platform: "Apple Music",
					Title:    "Apple Music link",
					EmbedURL: "",
				},
			},
		},
	}

	if !migrateSavedLinks(&state) {
		t.Fatal("expected stale Apple Music link to migrate")
	}

	link := state.Playlist.Links[0]
	if link.ID != "saved-apple" {
		t.Fatalf("id = %q, want saved-apple", link.ID)
	}
	if link.URL != appleMusicSampleURL {
		t.Fatalf("url = %q, want original URL", link.URL)
	}
	if link.Name != "Apple Music playlist" {
		t.Fatalf("name = %q, want Apple Music playlist", link.Name)
	}
	if link.Title != "Apple Music playlist" {
		t.Fatalf("title = %q, want Apple Music playlist", link.Title)
	}
	if link.EmbedURL != appleMusicSampleEmbedURL {
		t.Fatalf("embed URL = %q, want %q", link.EmbedURL, appleMusicSampleEmbedURL)
	}
}

func TestMigrateSavedAppleMusicLinkKeepsCustomName(t *testing.T) {
	state := State{
		Playlist: Playlist{
			Links: []Link{
				{
					ID:       "saved-apple",
					Name:     "Roadtrip Country",
					URL:      appleMusicSampleURL,
					Platform: "Apple Music",
					Title:    "Apple Music link",
					EmbedURL: "",
				},
			},
		},
	}

	if !migrateSavedLinks(&state) {
		t.Fatal("expected stale Apple Music link to migrate")
	}

	link := state.Playlist.Links[0]
	if link.Name != "Roadtrip Country" {
		t.Fatalf("name = %q, want custom name", link.Name)
	}
	if link.Title != "Apple Music playlist" {
		t.Fatalf("title = %q, want Apple Music playlist", link.Title)
	}
	if link.EmbedURL != appleMusicSampleEmbedURL {
		t.Fatalf("embed URL = %q, want %q", link.EmbedURL, appleMusicSampleEmbedURL)
	}
}

func TestYoutubeListID(t *testing.T) {
	tests := []struct {
		name, urlStr, platform, want string
	}{
		{"youtube playlist page", "https://www.youtube.com/playlist?list=PLMC9KNkIncKtPzgY-5rmhvj7fax8fdxoj", "YouTube", "PLMC9KNkIncKtPzgY-5rmhvj7fax8fdxoj"},
		{"watch url with list", "https://www.youtube.com/watch?v=abc123&list=PLxyz_78-9", "YouTube", "PLxyz_78-9"},
		{"youtube music playlist", "https://music.youtube.com/playlist?list=OLAK5uy_kkl6WOHzhBtwd6uwynLBpSC9RTMCEbwlM", "YouTube Music", "OLAK5uy_kkl6WOHzhBtwd6uwynLBpSC9RTMCEbwlM"},
		{"mix has no listing", "https://www.youtube.com/watch?v=abc123&list=RDabc123", "YouTube", ""},
		{"liked music is private", "https://music.youtube.com/playlist?list=LM", "YouTube Music", ""},
		{"plain video", "https://www.youtube.com/watch?v=abc123", "YouTube", ""},
		{"unsafe id", "https://www.youtube.com/playlist?list=PL%22evil", "YouTube", ""},
		{"not youtube", "https://open.spotify.com/playlist/xyz?list=PLabc", "Spotify", ""},
	}
	for _, tt := range tests {
		got := youtubeListID(Link{URL: tt.urlStr, Platform: tt.platform})
		if got != tt.want {
			t.Errorf("%s: youtubeListID = %q, want %q", tt.name, got, tt.want)
		}
	}
}
