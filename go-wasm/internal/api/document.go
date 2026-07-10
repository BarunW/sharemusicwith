package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"strings"
	"unicode"

	"connect-with-playlist-wasm/internal/state"
	"connect-with-playlist-wasm/internal/store"
)

const (
	homeTitle         = "ShareMusicWith | Create & Discover Playlist Pages"
	homeDescription   = "Create one beautiful page for your Spotify, YouTube Music, Apple Music, and SoundCloud playlists, share it anywhere, and discover music from real people."
	createTitle       = "Create a Shareable Playlist Page | ShareMusicWith"
	createDescription = "Build a personal page for your Spotify, YouTube Music, Apple Music, and SoundCloud playlists. Publish free in seconds, with no account required."
)

type documentRenderer struct {
	template *template.Template
	siteURL  string
}

type documentLink struct {
	ID       string
	Name     string
	URL      string
	Platform string
}

type pageDocument struct {
	Title         string
	Description   string
	Robots        string
	CanonicalURL  string
	OGType        string
	OGImageURL    string
	Social        bool
	JSONLD        template.JS
	BodyClass     string
	ShellClass    string
	Mode          string
	Theme         string
	Density       string
	Handle        string
	DisplayName   string
	Bio           string
	PlaylistTitle string
	AvatarInitial string
	Links         []documentLink
}

func newDocumentRenderer(indexPath, rawSiteURL string) (*documentRenderer, error) {
	siteURL, err := normalizeSiteURL(rawSiteURL)
	if err != nil {
		return nil, err
	}
	source, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read index template: %w", err)
	}
	tmpl, err := template.New("index.html").Parse(string(source))
	if err != nil {
		return nil, fmt.Errorf("parse index template: %w", err)
	}
	return &documentRenderer{template: tmpl, siteURL: siteURL}, nil
}

func normalizeSiteURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("SITE_URL must be an absolute http(s) origin")
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("SITE_URL must not contain a path, query, fragment, or credentials")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (d *documentRenderer) baseDocument() pageDocument {
	return pageDocument{
		Robots:        "index,follow,max-image-preview:large",
		OGType:        "website",
		OGImageURL:    d.siteURL + "/assets/social-card.png",
		Social:        true,
		Mode:          "home",
		Theme:         "storybook",
		Density:       "focused",
		Handle:        "yourname",
		DisplayName:   "Your Name",
		Bio:           "Share your favorite playlists in one beautiful page.",
		PlaylistTitle: "Your playlists",
		AvatarInitial: "Y",
	}
}

func (d *documentRenderer) homeDocument() pageDocument {
	doc := d.baseDocument()
	doc.Title = homeTitle
	doc.Description = homeDescription
	doc.CanonicalURL = d.siteURL + "/"
	doc.JSONLD = marshalJSONLD(map[string]any{
		"@context":    "https://schema.org",
		"@type":       "WebSite",
		"name":        "ShareMusicWith",
		"url":         doc.CanonicalURL,
		"description": doc.Description,
	})
	return doc
}

func (d *documentRenderer) createDocument() pageDocument {
	doc := d.baseDocument()
	doc.Title = createTitle
	doc.Description = createDescription
	doc.CanonicalURL = d.siteURL + "/create"
	doc.Mode = "create"
	doc.JSONLD = marshalJSONLD(map[string]any{
		"@context":            "https://schema.org",
		"@type":               "WebApplication",
		"name":                "ShareMusicWith",
		"url":                 doc.CanonicalURL,
		"description":         doc.Description,
		"applicationCategory": "MultimediaApplication",
		"operatingSystem":     "Any",
		"offers": map[string]any{
			"@type":         "Offer",
			"price":         "0",
			"priceCurrency": "USD",
		},
	})
	return doc
}

func (d *documentRenderer) createdDocument() pageDocument {
	doc := d.baseDocument()
	doc.Title = "Page Published | ShareMusicWith"
	doc.Description = "Your ShareMusicWith page has been published."
	doc.Robots = "noindex,follow,noarchive"
	doc.Social = false
	doc.Mode = "created"
	return doc
}

func (d *documentRenderer) editDocument() pageDocument {
	doc := d.baseDocument()
	doc.Title = "Edit Your Page | ShareMusicWith"
	doc.Description = "Edit your private ShareMusicWith playlist page."
	doc.Robots = "noindex,nofollow,noarchive"
	doc.Social = false
	doc.BodyClass = "view-only-body"
	doc.Mode = "edit"
	return doc
}

func (d *documentRenderer) notFoundDocument() pageDocument {
	doc := d.baseDocument()
	doc.Title = "Page Not Found | ShareMusicWith"
	doc.Description = "This ShareMusicWith page does not exist."
	doc.Robots = "noindex,nofollow,noarchive"
	doc.Social = false
	doc.BodyClass = "view-only-body"
	doc.ShellClass = "view-only"
	doc.Mode = "view"
	doc.Theme = "minimal"
	doc.Handle = "not-found"
	doc.DisplayName = "Page not found"
	doc.Bio = "This page does not exist."
	doc.PlaylistTitle = "Nothing to play here"
	doc.AvatarInitial = "?"
	return doc
}

func (d *documentRenderer) unavailableDocument() pageDocument {
	doc := d.notFoundDocument()
	doc.Title = "Page Temporarily Unavailable | ShareMusicWith"
	doc.Description = "This ShareMusicWith page is temporarily unavailable."
	doc.DisplayName = "Temporarily unavailable"
	doc.Bio = "Please try again shortly."
	doc.PlaylistTitle = "Could not load this page"
	doc.AvatarInitial = "!"
	return doc
}

func (d *documentRenderer) profileDocument(p *store.Playlist) pageDocument {
	doc := d.baseDocument()
	handle := compactText(p.Handle)
	displayName := compactText(p.State.User.DisplayName)
	if displayName == "" {
		displayName = "@" + handle
	}
	playlistTitle := compactText(p.State.Playlist.Title)
	if playlistTitle == "" {
		playlistTitle = "Playlists"
	}
	bio := compactText(p.State.User.Bio)
	description := bio
	if description == "" {
		description = fmt.Sprintf("Listen to %s, a playlist collection shared by %s on ShareMusicWith.", playlistTitle, displayName)
	}

	doc.Title = fmt.Sprintf("%s by @%s | ShareMusicWith", playlistTitle, handle)
	doc.Description = description
	doc.CanonicalURL = d.siteURL + "/@" + handle
	doc.OGType = "profile"
	doc.BodyClass = "view-only-body"
	doc.ShellClass = "view-only"
	doc.Mode = "view"
	doc.Theme = supportedTheme(p.State.Theme)
	doc.Density = supportedDensity(p.State.Density)
	doc.Handle = handle
	doc.DisplayName = displayName
	doc.Bio = bio
	doc.PlaylistTitle = playlistTitle
	doc.AvatarInitial = avatarInitial(displayName, handle)
	doc.Links = documentLinks(p.State.Playlist.Links)
	doc.JSONLD = profileJSONLD(doc, p.State.Playlist.Links)
	return doc
}

func documentLinks(links []state.Link) []documentLink {
	out := make([]documentLink, 0, len(links))
	for _, link := range links {
		if !strings.HasPrefix(link.URL, "https://") && !strings.HasPrefix(link.URL, "http://") {
			continue
		}
		name := compactText(link.Name)
		if name == "" {
			name = compactText(link.Title)
		}
		if name == "" {
			name = compactText(link.Platform)
		}
		if name == "" {
			name = "Playlist"
		}
		out = append(out, documentLink{
			ID:       link.ID,
			Name:     name,
			URL:      link.URL,
			Platform: compactText(link.Platform),
		})
	}
	return out
}

func profileJSONLD(doc pageDocument, links []state.Link) template.JS {
	items := make([]map[string]any, 0, len(links))
	position := 1
	for _, link := range links {
		if !strings.HasPrefix(link.URL, "https://") && !strings.HasPrefix(link.URL, "http://") {
			continue
		}
		name := compactText(link.Name)
		if name == "" {
			name = compactText(link.Title)
		}
		if name == "" {
			name = "Playlist"
		}
		items = append(items, map[string]any{
			"@type":    "ListItem",
			"position": position,
			"name":     name,
			"url":      link.URL,
		})
		position++
	}
	return marshalJSONLD(map[string]any{
		"@context":    "https://schema.org",
		"@type":       "ProfilePage",
		"url":         doc.CanonicalURL,
		"name":        doc.Title,
		"description": doc.Description,
		"mainEntity": map[string]any{
			"@type":         "Person",
			"name":          doc.DisplayName,
			"alternateName": "@" + doc.Handle,
			"description":   doc.Bio,
			"url":           doc.CanonicalURL,
		},
		"hasPart": map[string]any{
			"@type":           "ItemList",
			"name":            doc.PlaylistTitle,
			"itemListElement": items,
		},
	})
}

func marshalJSONLD(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	// encoding/json escapes HTML-significant characters, so user-owned strings
	// cannot terminate the application/ld+json script element.
	return template.JS(b)
}

func compactText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func avatarInitial(displayName, handle string) string {
	value := strings.TrimSpace(displayName)
	if value == "" {
		value = strings.TrimSpace(handle)
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return "?"
}

func supportedTheme(theme string) string {
	switch theme {
	case "storybook", "cyber", "nature", "minimal", "inkwash":
		return theme
	default:
		return "storybook"
	}
}

func supportedDensity(density string) string {
	if density == "compact" {
		return density
	}
	return "focused"
}
