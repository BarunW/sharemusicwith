package main

import (
	"strconv"
	"strings"
	"syscall/js"
)

// Mode is the current app screen, derived from the URL path.
type Mode int

const (
	ModeCreate  Mode = iota // "/create"      fresh draft (localStorage)
	ModeCreated             // "/created"     post-publish URLs
	ModeView                // "/@handle"     public read-only page
	ModeEdit                // "/@handle/edit/<token>" prefilled editor (autosave)
	ModeHome                // "/"            Spotify-style discovery feed
)

type route struct {
	mode      Mode
	handle    string
	editToken string
}

// parseRoute maps a pathname to a route. App routes always start with "@";
// anything unrecognized falls back to the creation screen.
func parseRoute(pathname string) route {
	p := strings.Trim(pathname, "/")
	switch {
	case p == "":
		return route{mode: ModeHome}
	case p == "create":
		return route{mode: ModeCreate}
	case p == "created":
		return route{mode: ModeCreated}
	case strings.HasPrefix(p, "@"):
		segs := strings.Split(p, "/")
		h := strings.TrimPrefix(segs[0], "@")
		if len(segs) >= 3 && segs[1] == "edit" {
			return route{mode: ModeEdit, handle: h, editToken: segs[2]}
		}
		if len(segs) == 1 {
			return route{mode: ModeView, handle: h}
		}
	}
	return route{mode: ModeHome}
}

func (a *App) currentRoute() route {
	return parseRoute(a.window.Get("location").Get("pathname").String())
}

// isAppRoute reports whether a pathname is one the SPA renders in-place. Used by
// the link interceptor so only these get client-side routing; everything else
// (static assets, unknown paths) falls through to a normal browser navigation.
func isAppRoute(pathname string) bool {
	p := strings.Trim(pathname, "/")
	return p == "" || p == "create" || p == "created" || strings.HasPrefix(p, "@")
}

func (a *App) navigate(path string) {
	a.window.Get("history").Call("pushState", nil, "", path)
	a.applyRoute(parseRoute(path))
	// A fresh screen should start at the top — without this an in-app jump keeps
	// the previous page's scroll offset (e.g. a scrolled discovery feed → a view
	// page that then opens mid-way down).
	a.window.Call("scrollTo", 0, 0)
}

// applyRoute is the single dispatcher that loads state for the active mode and
// renders. Server fetches (view/edit) run in a goroutine so the JS thread stays
// responsive; the result re-renders when it arrives.
func (a *App) applyRoute(r route) {
	a.mode = r.mode
	a.routeHandle = r.handle
	a.editToken = r.editToken
	a.activeWebviewLinkID = ""
	a.loading = false

	switch r.mode {
	case ModeHome:
		a.isViewOnly = false
		// Home doesn't go through render()/renderViewMode(), so clear any
		// view-only state left over from a /@handle or /create#view page —
		// otherwise .app-shell.view-only (height:100vh; overflow:hidden)
		// lingers and the discovery feed can't scroll.
		toggleClass(a.elements.AppShell, "view-only", false)
		toggleClass(a.elements.AppShell, "has-webview", false)
		toggleClass(a.document.Get("body"), "view-only-body", false)
		a.renderModeChrome()
		a.elements.AppShell.Get("dataset").Set("theme", "storybook")
		a.elements.AppShell.Get("dataset").Set("density", "focused")
		a.renderDiscover()
	case ModeCreate:
		a.state = a.loadState()
		a.isViewOnly = a.window.Get("location").Get("hash").String() == "#view"
		a.hydrateForm()
		a.render()
	case ModeCreated:
		if a.published == nil {
			a.navigate("/")
			return
		}
		a.isViewOnly = false
		a.render()
		a.renderCreatedPanel()
	case ModeView:
		a.isViewOnly = true
		a.loading = true
		a.state = loadingState("Loading page…")
		a.render()
		go a.loadServerState("/api/playlists/"+a.routeHandle, false)
	case ModeEdit:
		a.isViewOnly = a.window.Get("location").Get("hash").String() == "#view"
		a.loading = true
		a.state = loadingState("Loading editor…")
		a.hydrateForm()
		a.render()
		go a.loadServerState("/api/playlists/"+a.routeHandle+"/edit/"+a.editToken, true)
	}
}

func loadingState(title string) State {
	return State{
		Playlist: Playlist{Title: title},
		Theme:    "storybook",
		Density:  "focused",
	}
}

func (a *App) loadServerState(path string, editable bool) {
	st, status, err := a.apiGetState(path)
	if err != nil {
		a.showLoadError("Could not load this page.")
		return
	}
	switch status {
	case 200:
		// continue
	case 403:
		a.showLoadError("This edit link is not valid.")
		return
	case 404:
		a.showLoadError("This page does not exist.")
		return
	default:
		a.showLoadError("Could not load this page.")
		return
	}
	a.state = st
	a.loading = false
	a.hydrateForm()
	a.render()
	if !editable {
		go a.apiPostEvent(a.routeHandle, "page_view", "", "")
	}
}

func (a *App) showLoadError(message string) {
	a.state = State{
		User:    User{DisplayName: "Hmm…", Bio: message},
		Theme:   "minimal",
		Density: "focused",
	}
	a.isViewOnly = true
	a.loading = false
	a.render()
}

// publish gathers the draft, claims a handle, and shows the created screen.
func (a *App) publish() {
	a.syncFormToState()
	h := normalizeHandle(a.elements.Handle.Get("value").String())
	if h == "" {
		a.setStatus("Add a handle before publishing.")
		return
	}
	a.state.User.Handle = h
	a.elements.PublishButton.Set("disabled", true)
	a.setStatus("Publishing…")
	go func() {
		res, status, err := a.apiPostPublish(a.state, h)
		a.elements.PublishButton.Set("disabled", false)
		if err != nil {
			a.setStatus("Could not publish. Check your connection.")
			return
		}
		if status == 422 {
			a.setStatus("That handle is not allowed. Try another.")
			return
		}
		if status != 200 && status != 201 {
			a.setStatus("Could not publish (error " + strconv.Itoa(status) + ").")
			return
		}
		a.published = &res
		a.navigate("/created")
	}()
}

func (a *App) editPath() string {
	return "/api/playlists/" + a.routeHandle + "/edit/" + a.editToken
}

// scheduleServerSave debounces autosave in edit mode (re-arming a single timer).
func (a *App) scheduleServerSave() {
	if a.saveTimerSet {
		a.window.Call("clearTimeout", a.saveTimer)
	}
	a.setSaveStatus("Saving…")
	a.saveTimer = a.window.Call("setTimeout", a.saveCb, 800)
	a.saveTimerSet = true
}

// scheduleHandleCheck debounces the live availability check in create mode.
func (a *App) scheduleHandleCheck() {
	if a.handleCheckTimerSet {
		a.window.Call("clearTimeout", a.handleCheckTimer)
	}
	a.handleCheckTimer = a.window.Call("setTimeout", a.handleCheckCb, 400)
	a.handleCheckTimerSet = true
}

func (a *App) renderModeChrome() {
	dataset := a.elements.AppShell.Get("dataset")
	dataset.Set("mode", modeName(a.mode))
	// data-booting drives the full-screen boot spinner. The inline script in
	// index.html sets it on first load; we keep it up while a view/edit page is
	// still fetching its real state (a.loading) so the empty loading state isn't
	// shown as the create-page placeholder skin — and set it on in-app
	// navigations too, since the inline script only runs on the initial load.
	if a.loading {
		dataset.Set("booting", "1")
	} else {
		dataset.Delete("booting")
	}
}

func modeName(m Mode) string {
	switch m {
	case ModeHome:
		return "home"
	case ModeCreated:
		return "created"
	case ModeView:
		return "view"
	case ModeEdit:
		return "edit"
	default:
		return "create"
	}
}

func (a *App) renderCreatedPanel() {
	if a.published == nil {
		return
	}
	origin := a.window.Get("location").Get("origin").String()
	setText(a.elements.PublicURLValue, origin+a.published.PublicURL)
	setText(a.elements.EditURLValue, origin+a.published.EditURL)
}

func (a *App) syncFormToState() {
	a.state.User.DisplayName = strings.TrimSpace(a.elements.DisplayName.Get("value").String())
	a.state.User.Handle = normalizeHandle(a.elements.Handle.Get("value").String())
	a.state.User.Bio = strings.TrimSpace(a.elements.Bio.Get("value").String())
	a.state.Playlist.Title = strings.TrimSpace(a.elements.PageTitle.Get("value").String())
}

func (a *App) setSaveStatus(message string) {
	setText(a.elements.SaveStatus, message)
}

func (a *App) setHandleHint(message, cls string) {
	setText(a.elements.HandleHint, message)
	classList := a.elements.HandleHint.Get("classList")
	classList.Call("remove", "is-available", "is-taken")
	if cls != "" {
		classList.Call("add", cls)
	}
}

// renderDiscover fetches each discovery section and fills its card grid. Each
// section loads independently in its own goroutine so a slow one doesn't block
// the others (net/http on wasm uses fetch; DOM writes resume on the JS thread).
func (a *App) renderDiscover() {
	sections := []struct{ key, grid string }{
		{"trending", "#gridTrending"},
		{"today", "#gridToday"},
		{"popular", "#gridPopular"},
		{"new", "#gridNew"},
	}
	for _, s := range sections {
		grid := a.document.Call("querySelector", s.grid)
		if isNil(grid) {
			continue
		}
		a.setDiscoverPlaceholder(grid, "Loading…")
		key, g := s.key, grid
		go func() {
			items, err := a.apiGetDiscover(key)
			if err != nil {
				a.setDiscoverPlaceholder(g, "Couldn't load right now.")
				return
			}
			a.fillDiscoverGrid(g, items, key)
		}()
	}
}

func (a *App) setDiscoverPlaceholder(grid js.Value, text string) {
	grid.Call("replaceChildren")
	p := a.el("p")
	p.Get("classList").Call("add", "discover-empty")
	setText(p, text)
	grid.Call("append", p)
}

func (a *App) fillDiscoverGrid(grid js.Value, items []discoverItem, section string) {
	grid.Call("replaceChildren")
	if len(items) == 0 {
		a.setDiscoverPlaceholder(grid, "Nothing here yet.")
		return
	}
	for _, it := range items {
		grid.Call("append", a.discoverCard(it, section))
	}
}

// discoverCard builds one horizontal playlist card linking to its public page.
// It's a plain anchor so it works without JS wiring and right-click/open-in-
// new-tab behave. Left: a deterministic gradient-aura cover (seeded from the
// handle, no stored image) with a section pill. Right: title, @handle, and a
// footer with a play glyph, the section metric, and the track count.
func (a *App) discoverCard(it discoverItem, section string) js.Value {
	card := a.el("a")
	card.Get("classList").Call("add", "discover-card")
	card.Set("href", "/@"+it.Handle)

	cover := a.el("span")
	cover.Get("classList").Call("add", "discover-card-cover")
	cover.Get("style").Set("background", auraStyle(it.Handle))

	// Vinyl motif: a record with a two-tone pressing label seeded from the handle
	// (see vinylNode). Cream center shows the initials; the accent band shows the
	// platform. Gives each cover real identity, not just color.
	vinylWrap := a.el("span")
	vinylWrap.Get("classList").Call("add", "discover-card-vinyl")
	vinylWrap.Call("setAttribute", "aria-hidden", "true")
	vinylWrap.Call("append", a.vinylNode(
		it.Handle,
		getInitials(fallback(it.Title, it.Handle)),
		"",
		strings.ToUpper(platformLabel(it.Platform)),
	))
	cover.Call("append", vinylWrap)

	pill := a.el("span")
	pill.Get("classList").Call("add", "discover-card-pill")
	setText(pill, sectionPill(section))
	cover.Call("append", pill)

	body := a.el("span")
	body.Get("classList").Call("add", "discover-card-body")

	title := a.el("strong")
	title.Get("classList").Call("add", "discover-card-title")
	setText(title, fallback(it.Title, "Untitled playlist"))

	handle := a.el("span")
	handle.Get("classList").Call("add", "discover-card-handle")
	setText(handle, "@"+it.Handle)

	meta := a.el("span")
	meta.Get("classList").Call("add", "discover-card-meta")
	meta.Call("append", handle)
	if plat := platformLabel(it.Platform); plat != "" {
		tag := a.el("span")
		tag.Get("classList").Call("add", "discover-card-tag")
		setText(tag, plat)
		meta.Call("append", tag)
	}

	foot := a.el("span")
	foot.Get("classList").Call("add", "discover-card-foot")

	play := a.el("span")
	play.Get("classList").Call("add", "discover-card-play")
	play.Call("setAttribute", "aria-hidden", "true")

	plays := a.el("span")
	plays.Get("classList").Call("add", "discover-card-plays")
	setText(plays, discoverMetric(it, section))

	tracks := a.el("span")
	tracks.Get("classList").Call("add", "discover-card-tracks")
	setText(tracks, trackCount(it.LinkCount))

	foot.Call("append", play, plays, tracks)
	body.Call("append", title, meta, foot)
	card.Call("append", cover, body)
	return card
}

// vinylNode builds the reusable record element used on discover covers and the
// profile avatar: a grooved disc with a two-tone pressing label (cream top, a
// seeded accent band via the --vinyl-accent custom property), a spindle hole,
// and centered initials. top/band are optional small wording lines (empty =
// omitted) — e.g. the platform on cards, the @handle on the avatar.
func (a *App) vinylNode(seed, initials, top, band string) js.Value {
	vinyl := a.el("span")
	vinyl.Get("classList").Call("add", "vinyl")

	label := a.el("span")
	label.Get("classList").Call("add", "vinyl-label")
	label.Get("style").Call("setProperty", "--vinyl-accent", vinylAccent(seed))

	if top != "" {
		t := a.el("span")
		t.Get("classList").Call("add", "vinyl-brand")
		setText(t, top)
		label.Call("append", t)
	}

	ini := a.el("span")
	ini.Get("classList").Call("add", "vinyl-initials")
	setText(ini, initials)
	label.Call("append", ini)

	hole := a.el("span")
	hole.Get("classList").Call("add", "vinyl-hole")
	label.Call("append", hole)

	if band != "" {
		bd := a.el("span")
		bd.Get("classList").Call("add", "vinyl-band")
		setText(bd, band)
		label.Call("append", bd)
	}

	vinyl.Call("append", label)
	return vinyl
}

// sectionPill is the editorial label shown on a card, derived from the feed
// section (no per-playlist tag is stored).
func sectionPill(section string) string {
	switch section {
	case "today":
		return "HOT TODAY"
	case "popular":
		return "POPULAR"
	case "new":
		return "NEW"
	default:
		return "TRENDING"
	}
}

// platformLabel maps a stored platform key to a display label for the card tag.
// Empty (unknown/unset) platforms render no chip rather than a placeholder.
func platformLabel(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "":
		return ""
	case "spotify":
		return "Spotify"
	case "apple", "apple_music", "applemusic":
		return "Apple Music"
	case "youtube", "youtube_music", "yt":
		return "YouTube"
	case "soundcloud":
		return "SoundCloud"
	case "tidal":
		return "Tidal"
	case "bandcamp":
		return "Bandcamp"
	case "mixed":
		return "Mixed"
	default:
		return strings.ToUpper(p[:1]) + p[1:]
	}
}

func trackCount(n int) string {
	if n == 1 {
		return "1 track"
	}
	return strconv.Itoa(n) + " tracks"
}

func discoverName(it discoverItem) string {
	if strings.TrimSpace(it.DisplayName) != "" {
		return it.DisplayName
	}
	return it.Handle
}

func discoverMetric(it discoverItem, section string) string {
	switch section {
	case "today":
		return humanCount(it.Views24h) + plural(it.Views24h, " view today", " views today")
	case "popular":
		return humanCount(it.UniqueViews) + plural(it.UniqueViews, " listener", " listeners")
	case "new":
		return "Just added"
	default: // trending
		return humanCount(it.EngagedPlays) + plural(it.EngagedPlays, " play", " plays")
	}
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// humanCount renders large counts compactly (1.2K, 3.4M).
func humanCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64) + "M"
	case n >= 1_000:
		return strconv.FormatFloat(float64(n)/1_000, 'f', 1, 64) + "K"
	default:
		return strconv.FormatInt(n, 10)
	}
}

func (a *App) copyToClipboard(text string, button js.Value) {
	navigator := a.window.Get("navigator")
	if !isNil(navigator) {
		if clip := navigator.Get("clipboard"); !isNil(clip) {
			clip.Call("writeText", text)
		}
	}
	original := button.Get("textContent").String()
	setText(button, "Copied")
	var cb js.Func
	cb = js.FuncOf(func(js.Value, []js.Value) any {
		setText(button, original)
		cb.Release()
		return nil
	})
	a.window.Call("setTimeout", cb, 1200)
}
