// Package state defines the playlist page document model shared by the WASM
// client (cmd/app) and the server (cmd/server). It is intentionally
// stdlib-only so it stays safe to compile for GOOS=js/GOARCH=wasm.
package state

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// User is the public profile shown at the top of a page.
type User struct {
	DisplayName string `json:"displayName"`
	Handle      string `json:"handle"`
	Bio         string `json:"bio"`
}

// Link is a single playlist/embed entry.
type Link struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Platform string `json:"platform"`
	Title    string `json:"title"`
	EmbedURL string `json:"embedUrl"`
}

// Playlist groups the page's links under a heading.
type Playlist struct {
	Title string `json:"title"`
	Links []Link `json:"links"`
}

// State is the whole document persisted per page.
type State struct {
	User     User     `json:"user"`
	Playlist Playlist `json:"playlist"`
	Theme    string   `json:"theme"`
	Density  string   `json:"density"`
}

// Field length caps (mirror the HTML maxlength attributes plus sane limits so
// a hostile client cannot bloat the JSONB column).
const (
	MaxDisplayName = 42
	MaxHandle      = 32
	MaxBio         = 150
	MaxTitle       = 56
	MaxLinkName    = 56
	MaxLinks       = 100
	MaxURL         = 2048
)

// Validate enforces the field caps and that any URLs use http(s) schemes,
// which blocks javascript:/data: URIs from ever reaching an iframe src.
func Validate(s State) error {
	if utf8.RuneCountInString(s.User.DisplayName) > MaxDisplayName {
		return fmt.Errorf("display name exceeds %d characters", MaxDisplayName)
	}
	if utf8.RuneCountInString(s.User.Handle) > MaxHandle {
		return fmt.Errorf("handle exceeds %d characters", MaxHandle)
	}
	if utf8.RuneCountInString(s.User.Bio) > MaxBio {
		return fmt.Errorf("bio exceeds %d characters", MaxBio)
	}
	if utf8.RuneCountInString(s.Playlist.Title) > MaxTitle {
		return fmt.Errorf("title exceeds %d characters", MaxTitle)
	}
	if len(s.Playlist.Links) > MaxLinks {
		return fmt.Errorf("too many links (max %d)", MaxLinks)
	}
	for i, l := range s.Playlist.Links {
		if utf8.RuneCountInString(l.Name) > MaxLinkName {
			return fmt.Errorf("link %d name too long", i)
		}
		if len(l.URL) > MaxURL || len(l.EmbedURL) > MaxURL {
			return fmt.Errorf("link %d url too long", i)
		}
		if l.URL != "" && !isHTTP(l.URL) {
			return fmt.Errorf("link %d url must be http(s)", i)
		}
		if l.EmbedURL != "" && !strings.HasPrefix(l.EmbedURL, "https://") {
			return fmt.Errorf("link %d embed url must be https", i)
		}
	}
	return nil
}

func isHTTP(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}
