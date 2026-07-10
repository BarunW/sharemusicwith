package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"html"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connect-with-playlist-wasm/internal/config"
	"connect-with-playlist-wasm/internal/state"
	"connect-with-playlist-wasm/internal/store"
)

type fakePlaylistStore struct {
	playlist   *store.Playlist
	getErr     error
	sitemap    []store.SitemapPage
	sitemapErr error
}

func (f *fakePlaylistStore) Ping(context.Context) error { return nil }
func (f *fakePlaylistStore) CreatePlaylist(context.Context, string, state.State, []byte) (string, error) {
	return "test", nil
}
func (f *fakePlaylistStore) GetByHandle(context.Context, string) (*store.Playlist, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.playlist == nil {
		return nil, store.ErrNotFound
	}
	return f.playlist, nil
}
func (f *fakePlaylistStore) IncrementViewCount(context.Context, string) (int64, error) {
	return 0, nil
}
func (f *fakePlaylistStore) GetForEdit(context.Context, string, string) (*store.Playlist, error) {
	return f.playlist, nil
}
func (f *fakePlaylistStore) UpdateByToken(context.Context, string, string, state.State) error {
	return nil
}
func (f *fakePlaylistStore) HandleExists(context.Context, string) (bool, error) {
	return false, nil
}
func (f *fakePlaylistStore) InsertEvent(context.Context, string, string, string, string, string, string, []byte, []byte, bool) error {
	return nil
}
func (f *fakePlaylistStore) ListRankings(context.Context, string, int) ([]store.Ranking, error) {
	return nil, nil
}
func (f *fakePlaylistStore) ListSitemapPages(context.Context) ([]store.SitemapPage, error) {
	return f.sitemap, f.sitemapErr
}

func newSEOTestRouter(t *testing.T, st playlistStore) http.Handler {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(st, config.Config{
		StaticDir: root,
		SiteURL:   "https://example.test",
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return router
}

func request(t *testing.T, h http.Handler, path, userAgent string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if userAgent != "" {
		r.Header.Set("User-Agent", userAgent)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHomeAndCreateDocumentsAreDistinct(t *testing.T) {
	router := newSEOTestRouter(t, &fakePlaylistStore{})
	home := request(t, router, "/", "")
	create := request(t, router, "/create", "")

	if home.Code != http.StatusOK || create.Code != http.StatusOK {
		t.Fatalf("statuses home=%d create=%d", home.Code, create.Code)
	}
	homeBody, createBody := home.Body.String(), create.Body.String()
	for _, body := range []string{homeBody, createBody} {
		if strings.Contains(body, "dreamlistener") || strings.Contains(body, "A small public page") {
			t.Fatal("route document leaked demo-profile content")
		}
	}
	if !strings.Contains(homeBody, "<title>"+html.EscapeString(homeTitle)+"</title>") ||
		!strings.Contains(homeBody, `href="https://example.test/"`) ||
		!strings.Contains(homeBody, homeDescription) ||
		!strings.Contains(homeBody, `property="og:image" content="https://example.test/assets/social-card.png"`) {
		t.Fatal("homepage metadata is incomplete")
	}
	if !strings.Contains(createBody, "<title>"+createTitle+"</title>") ||
		!strings.Contains(createBody, `href="https://example.test/create"`) ||
		!strings.Contains(createBody, createDescription) {
		t.Fatal("create metadata is incomplete")
	}
	if homeBody == createBody {
		t.Fatal("home and create responses must not be identical")
	}
}

func TestPublicProfileDocumentUsesStoredContentAndEscapesIt(t *testing.T) {
	st := &fakePlaylistStore{playlist: &store.Playlist{
		Handle: "barunw14",
		State: state.State{
			User: state.User{
				DisplayName: `Barun & Co`,
				Bio:         `A "bold" <mix> & more`,
			},
			Playlist: state.Playlist{
				Title: "Weekend Rotation",
				Links: []state.Link{{
					ID:       "link-1",
					Name:     "Night & Day",
					URL:      "https://open.spotify.com/playlist/example",
					Platform: "Spotify",
				}},
			},
			Theme:   "inkwash",
			Density: "compact",
		},
	}}
	router := newSEOTestRouter(t, st)
	w := request(t, router, "/@barunw14", "Mozilla/5.0")
	body := w.Body.String()

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, body)
	}
	checks := []string{
		"<title>Weekend Rotation by @barunw14 | ShareMusicWith</title>",
		`content="A &#34;bold&#34; &lt;mix&gt; &amp; more"`,
		`href="https://example.test/@barunw14"`,
		`data-theme="inkwash"`,
		`data-density="compact"`,
		`id="previewHandle">@barunw14`,
		`Night &amp; Day`,
	}
	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("profile response missing %q", check)
		}
	}
	if strings.Contains(body, "<mix>") || strings.Contains(body, "dreamlistener") {
		t.Fatal("profile response contains unsafe or demo content")
	}

	start := strings.Index(body, `<script type="application/ld+json">`)
	if start < 0 {
		t.Fatal("missing JSON-LD")
	}
	start += len(`<script type="application/ld+json">`)
	end := strings.Index(body[start:], "</script>")
	if end < 0 {
		t.Fatal("unterminated JSON-LD")
	}
	var jsonLD map[string]any
	if err := json.Unmarshal([]byte(html.UnescapeString(body[start:start+end])), &jsonLD); err != nil {
		t.Fatalf("invalid JSON-LD: %v", err)
	}
	if jsonLD["@type"] != "ProfilePage" {
		t.Fatalf("JSON-LD type=%v", jsonLD["@type"])
	}
}

func TestCrawlerAndBrowserReceiveSameProfileHTML(t *testing.T) {
	st := &fakePlaylistStore{playlist: &store.Playlist{
		Handle: "barunw14",
		State: state.State{
			User:     state.User{DisplayName: "Barun", Bio: "Music I keep returning to."},
			Playlist: state.Playlist{Title: "Weekend Rotation"},
		},
	}}
	router := newSEOTestRouter(t, st)
	bot := request(t, router, "/@barunw14", "Mozilla/5.0 (compatible; Googlebot/2.1)")
	browser := request(t, router, "/@barunw14", "Mozilla/5.0 Chrome/124 Safari/537.36")
	if bot.Code != browser.Code || bot.Body.String() != browser.Body.String() {
		t.Fatal("crawler and browser responses differ")
	}
}

func TestProfileErrorsAndPrivateRoutesAreNoIndex(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		store  *fakePlaylistStore
		status int
		secret string
	}{
		{"missing profile", "/@missing", &fakePlaylistStore{}, http.StatusNotFound, ""},
		{"invalid profile", "/@---", &fakePlaylistStore{}, http.StatusNotFound, ""},
		{"store unavailable", "/@barunw14", &fakePlaylistStore{getErr: errors.New("db down")}, http.StatusServiceUnavailable, ""},
		{"created", "/created", &fakePlaylistStore{}, http.StatusOK, ""},
		{"private edit", "/@barunw14/edit/super-secret", &fakePlaylistStore{}, http.StatusOK, "super-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := request(t, newSEOTestRouter(t, tt.store), tt.path, "")
			if w.Code != tt.status {
				t.Fatalf("status=%d want=%d", w.Code, tt.status)
			}
			if !strings.Contains(w.Header().Get("X-Robots-Tag"), "noindex") ||
				!strings.Contains(w.Body.String(), `content="noindex`) {
				t.Fatal("response must be noindex in both header and document")
			}
			if tt.secret != "" && strings.Contains(w.Body.String(), tt.secret) {
				t.Fatal("edit token leaked into response body")
			}
		})
	}
}

func TestProfileCanonicalRedirect(t *testing.T) {
	router := newSEOTestRouter(t, &fakePlaylistStore{})
	w := request(t, router, "/@BarunW14?utm_source=test", "")
	if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != "/@barunw14" {
		t.Fatalf("status=%d location=%q", w.Code, w.Header().Get("Location"))
	}
}

func TestRobotsAndSitemap(t *testing.T) {
	updated := time.Date(2026, 7, 10, 12, 30, 0, 0, time.FixedZone("IST", 5*60*60+30*60))
	st := &fakePlaylistStore{sitemap: []store.SitemapPage{{Handle: "barunw14", UpdatedAt: updated}}}
	router := newSEOTestRouter(t, st)

	robots := request(t, router, "/robots.txt", "")
	if robots.Code != http.StatusOK ||
		!strings.Contains(robots.Body.String(), "Disallow: /*/edit/") ||
		!strings.Contains(robots.Body.String(), "Sitemap: https://example.test/sitemap.xml") {
		t.Fatalf("unexpected robots.txt: %s", robots.Body.String())
	}

	sitemap := request(t, router, "/sitemap.xml", "")
	if sitemap.Code != http.StatusOK {
		t.Fatalf("sitemap status=%d body=%s", sitemap.Code, sitemap.Body.String())
	}
	var parsed sitemapURLSet
	if err := xml.Unmarshal(sitemap.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid sitemap XML: %v", err)
	}
	body := sitemap.Body.String()
	for _, value := range []string{
		"https://example.test/</loc>",
		"https://example.test/create</loc>",
		"https://example.test/@barunw14</loc>",
		"2026-07-10T07:00:00Z",
	} {
		if !strings.Contains(body, value) {
			t.Errorf("sitemap missing %q", value)
		}
	}
	if strings.Contains(body, "/created") || strings.Contains(body, "/edit/") {
		t.Fatal("sitemap contains a private/transient route")
	}
}

func TestNewRouterRejectsInvalidSiteURL(t *testing.T) {
	_, err := NewRouter(&fakePlaylistStore{}, config.Config{
		StaticDir: filepath.Join("..", ".."),
		SiteURL:   "javascript:alert(1)",
	})
	if err == nil {
		t.Fatal("expected invalid SITE_URL to fail router construction")
	}
}

func TestSocialCardDimensions(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "social-card.png")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	config, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatalf("decode social card: %v", err)
	}
	if config.Width != 1200 || config.Height != 630 {
		t.Fatalf("social card=%dx%d, want 1200x630", config.Width, config.Height)
	}
}
