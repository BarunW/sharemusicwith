// Package youtube fetches playlist track listings from the YouTube Data API
// v3. It runs only on the server (cmd/server) so the API key never reaches the
// client. YouTube Music playlists share IDs with regular YouTube playlists
// (music.youtube.com wraps them in a "VL" prefix, which NormalizeID strips),
// so one client covers both. Auto-generated mixes ("RD..." list IDs seeded
// from a video) are not real playlists and come back as ErrNotFound.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DefaultBaseURL is the production Data API endpoint prefix.
const DefaultBaseURL = "https://www.googleapis.com/youtube/v3"

// Sentinel errors the HTTP handler maps onto response codes.
var (
	ErrInvalidID = errors.New("youtube: invalid playlist id")
	ErrNotFound  = errors.New("youtube: playlist not found")
	ErrQuota     = errors.New("youtube: api quota exceeded")
)

// Track is one playable entry of a playlist.
type Track struct {
	VideoID   string `json:"videoId"`
	Title     string `json:"title"`
	Artist    string `json:"artist,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Position  int64  `json:"position"`
}

// Playlist is a fetched listing. Total is the size the API reports, which can
// exceed len(Tracks) when private/deleted entries were skipped or the listing
// was cut at MaxTracks (Truncated).
type Playlist struct {
	ID        string  `json:"id"`
	Title     string  `json:"title,omitempty"`
	Channel   string  `json:"channel,omitempty"`
	Tracks    []Track `json:"tracks"`
	Total     int     `json:"total"`
	Truncated bool    `json:"truncated"`
}

// Client calls the Data API. BaseURL, HTTPClient, and MaxTracks get sane
// defaults from New and are exported so tests can point at a fake server.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	MaxTracks  int

	apiKey string
}

// New returns a client with production defaults. MaxTracks caps how many
// entries a single fetch pages through (50 per page = 1 quota unit each), so
// a hostile 5000-video playlist cannot drain the daily quota.
func New(apiKey string) *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		MaxTracks:  500,
		apiKey:     apiKey,
	}
}

// validID covers every real playlist ID shape (PL…, OLAK5uy_…, UU…, FL…);
// they are URL-safe base64-ish and well under 64 chars.
var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{2,64}$`)

// NormalizeID trims whitespace, strips the "VL" wrapper music.youtube.com
// puts around playlist IDs, and validates the charset/length.
func NormalizeID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "VLPL") || strings.HasPrefix(id, "VLOLAK") || strings.HasPrefix(id, "VLRD") {
		id = id[2:]
	}
	if !validID.MatchString(id) {
		return "", ErrInvalidID
	}
	return id, nil
}

// FetchPlaylist returns the playlist metadata and every track, paging through
// playlistItems until the end or MaxTracks. Private/deleted entries (which
// the API returns with no thumbnails and no owner) are skipped.
func (c *Client) FetchPlaylist(ctx context.Context, id string) (*Playlist, error) {
	id, err := NormalizeID(id)
	if err != nil {
		return nil, err
	}

	pl := &Playlist{ID: id, Tracks: []Track{}}
	if err := c.fetchMeta(ctx, pl); err != nil {
		return nil, err
	}

	pageToken := ""
	for {
		page, err := c.fetchItemsPage(ctx, id, pageToken)
		if err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			sn := it.Snippet
			if sn.ResourceID.VideoID == "" || len(sn.Thumbnails) == 0 {
				continue // deleted or private video
			}
			pl.Tracks = append(pl.Tracks, Track{
				VideoID:   sn.ResourceID.VideoID,
				Title:     sn.Title,
				Artist:    cleanArtist(sn.VideoOwnerChannelTitle),
				Thumbnail: pickThumbnail(sn.Thumbnails),
				Position:  sn.Position,
			})
		}
		pl.Total = page.PageInfo.TotalResults
		if page.NextPageToken == "" {
			break
		}
		if len(pl.Tracks) >= c.MaxTracks {
			pl.Truncated = true
			break
		}
		pageToken = page.NextPageToken
	}
	if len(pl.Tracks) > c.MaxTracks {
		pl.Tracks = pl.Tracks[:c.MaxTracks]
		pl.Truncated = true
	}
	return pl, nil
}

// fetchMeta fills Title/Channel from playlists.list, which doubles as the
// existence check: unknown IDs return 200 with an empty items array.
func (c *Client) fetchMeta(ctx context.Context, pl *Playlist) error {
	q := url.Values{
		"part": {"snippet"},
		"id":   {pl.ID},
		"key":  {c.apiKey},
	}
	var res struct {
		Items []struct {
			Snippet struct {
				Title        string `json:"title"`
				ChannelTitle string `json:"channelTitle"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, "/playlists", q, &res); err != nil {
		return err
	}
	if len(res.Items) == 0 {
		return ErrNotFound
	}
	pl.Title = res.Items[0].Snippet.Title
	pl.Channel = res.Items[0].Snippet.ChannelTitle
	return nil
}

type itemsPage struct {
	NextPageToken string `json:"nextPageToken"`
	PageInfo      struct {
		TotalResults int `json:"totalResults"`
	} `json:"pageInfo"`
	Items []struct {
		Snippet struct {
			Title                  string               `json:"title"`
			VideoOwnerChannelTitle string               `json:"videoOwnerChannelTitle"`
			Position               int64                `json:"position"`
			Thumbnails             map[string]thumbnail `json:"thumbnails"`
			ResourceID             struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
		} `json:"snippet"`
	} `json:"items"`
}

type thumbnail struct {
	URL string `json:"url"`
}

func (c *Client) fetchItemsPage(ctx context.Context, id, pageToken string) (*itemsPage, error) {
	q := url.Values{
		"part":       {"snippet"},
		"playlistId": {id},
		"maxResults": {"50"},
		"key":        {c.apiKey},
	}
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	var page itemsPage
	if err := c.getJSON(ctx, "/playlistItems", q, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("youtube api: %w", err)
	}
	defer resp.Body.Close()
	body := io.LimitReader(resp.Body, 4<<20)
	if resp.StatusCode != http.StatusOK {
		return apiError(resp.StatusCode, body)
	}
	if err := json.NewDecoder(body).Decode(out); err != nil {
		return fmt.Errorf("youtube api: bad response: %w", err)
	}
	return nil
}

// apiError maps a non-200 Data API response onto the sentinel errors. The
// body shape is {"error": {"code": n, "errors": [{"reason": "..."}]}}.
func apiError(status int, body io.Reader) error {
	var e struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.NewDecoder(body).Decode(&e)
	reason := ""
	if len(e.Error.Errors) > 0 {
		reason = e.Error.Errors[0].Reason
	}
	switch {
	case status == http.StatusNotFound:
		return ErrNotFound
	case reason == "quotaExceeded" || reason == "rateLimitExceeded":
		return ErrQuota
	case status == http.StatusForbidden && reason == "playlistForbidden":
		return ErrNotFound // private playlist: indistinguishable from absent for our purposes
	default:
		return fmt.Errorf("youtube api: status %d reason %q", status, reason)
	}
}

// cleanArtist turns an owning-channel name into an artist label: YouTube
// Music auto-channels are named "Artist - Topic".
func cleanArtist(channel string) string {
	return strings.TrimSpace(strings.TrimSuffix(channel, " - Topic"))
}

// pickThumbnail prefers a mid-size image; any present size beats none.
func pickThumbnail(t map[string]thumbnail) string {
	for _, k := range []string{"medium", "high", "default", "standard", "maxres"} {
		if th, ok := t[k]; ok && th.URL != "" {
			return th.URL
		}
	}
	for _, th := range t {
		if th.URL != "" {
			return th.URL
		}
	}
	return ""
}
