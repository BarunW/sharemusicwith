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
