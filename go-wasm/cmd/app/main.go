package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/url"
	"strings"
	"syscall/js"
	"time"
	"unicode"

	"connect-with-playlist-wasm/internal/state"
)

const (
	storageKey                = "connect-with-playlist-go-wasm-state-v1"
	spotifyPreviewRestoredKey = "connect-with-playlist-go-wasm-spotify-preview-restored-v1"
	responsivePlayerQuery     = "(max-width: 1040px)"
)

var themeCopy = map[string]string{
	"storybook": "Soft painted color, calm spacing, and a page that feels personal without becoming heavy.",
	"cyber":     "Rainy neon glass, sharp contrast, and a live-mix profile built for late-night sharing.",
	"nature":    "Layered forest texture, warm sunlight, and room for playlists to breathe.",
	"minimal":   "Quiet contrast, restrained texture, and a simple frame around the music.",
	"inkwash":   "Misty Jiangnan watercolor — soft ink wash, rice-paper warmth, and plum-blossom color.",
}

// The document model lives in internal/state so the client and server share one
// wire format. Aliases keep the existing State/Link/... references working.
type (
	User     = state.User
	Link     = state.Link
	Playlist = state.Playlist
	State    = state.State
)

type Elements struct {
	AppShell         js.Value
	PageForm         js.Value
	DisplayName      js.Value
	Handle           js.Value
	Bio              js.Value
	PageTitle        js.Value
	PlaylistName     js.Value
	PlaylistURL      js.Value
	AddLink          js.Value
	AddSpotify       js.Value
	ViewOnly         js.Value
	ExitView         js.Value
	PlaylistList     js.Value
	ThemeGrid        js.Value
	PhoneStage       js.Value
	PublicPage       js.Value
	PlaylistWebview  js.Value
	WebviewPlatform  js.Value
	WebviewTitle     js.Value
	WebviewFrame     js.Value
	CloseWebview     js.Value
	InlinePlayer     js.Value
	InlinePlatform   js.Value
	InlineTitle      js.Value
	InlineFrame      js.Value
	CloseInline      js.Value
	PreviewName      js.Value
	PreviewHandle    js.Value
	PreviewBio       js.Value
	PreviewTitle     js.Value
	Avatar           js.Value
	EmbedStack       js.Value
	ThemeNote        js.Value
	StatusText       js.Value
	PlaylistTemplate js.Value
	EmbedTemplate    js.Value
	PublishButton    js.Value
	HandleHint       js.Value
	SaveStatus       js.Value
	CreatedPanel     js.Value
	PublicURLValue   js.Value
	EditURLValue     js.Value
	CopyPublicURL    js.Value
	CopyEditURL      js.Value
	CreateAnother    js.Value
	DiscoverCreate   js.Value
}

type App struct {
	document            js.Value
	window              js.Value
	responsivePlayer    js.Value
	elements            Elements
	state               State
	isViewOnly          bool
	activeWebviewLinkID string
	lastTouchY          float64
	callbacks           []js.Func

	// Routing / server-backed modes.
	mode        Mode
	routeHandle string
	editToken   string
	published   *publishResult

	// Debounced autosave (edit mode) and live handle availability (create mode).
	saveCb              js.Func
	saveTimer           js.Value
	saveTimerSet        bool
	handleCheckCb       js.Func
	handleCheckTimer    js.Value
	handleCheckTimerSet bool
}

func main() {
	app := newApp()
	app.bindEvents()
	app.applyRoute(app.currentRoute())
	select {}
}

func newApp() *App {
	document := js.Global().Get("document")
	app := &App{
		document:         document,
		window:           js.Global(),
		responsivePlayer: js.Global().Call("matchMedia", responsivePlayerQuery),
	}
	app.elements = Elements{
		AppShell:         document.Call("querySelector", ".app-shell"),
		PageForm:         document.Call("querySelector", "#pageForm"),
		DisplayName:      document.Call("querySelector", "#displayName"),
		Handle:           document.Call("querySelector", "#handle"),
		Bio:              document.Call("querySelector", "#bio"),
		PageTitle:        document.Call("querySelector", "#pageTitle"),
		PlaylistName:     document.Call("querySelector", "#playlistName"),
		PlaylistURL:      document.Call("querySelector", "#playlistUrl"),
		AddLink:          document.Call("querySelector", "#addLink"),
		AddSpotify:       document.Call("querySelector", "#addSpotifyPreview"),
		ViewOnly:         document.Call("querySelector", "#viewOnly"),
		ExitView:         document.Call("querySelector", "#exitView"),
		PlaylistList:     document.Call("querySelector", "#playlistList"),
		ThemeGrid:        document.Call("querySelector", "#themeGrid"),
		PhoneStage:       document.Call("querySelector", ".phone-stage"),
		PublicPage:       document.Call("querySelector", "#publicPage"),
		PlaylistWebview:  document.Call("querySelector", "#playlistWebview"),
		WebviewPlatform:  document.Call("querySelector", "#webviewPlatform"),
		WebviewTitle:     document.Call("querySelector", "#webviewTitle"),
		WebviewFrame:     document.Call("querySelector", "#webviewFrame"),
		CloseWebview:     document.Call("querySelector", "#closeWebview"),
		InlinePlayer:     document.Call("querySelector", "#playlistInlinePlayer"),
		InlinePlatform:   document.Call("querySelector", "#inlinePlayerPlatform"),
		InlineTitle:      document.Call("querySelector", "#inlinePlayerTitle"),
		InlineFrame:      document.Call("querySelector", "#inlinePlayerFrame"),
		CloseInline:      document.Call("querySelector", "#closeInlinePlayer"),
		PreviewName:      document.Call("querySelector", "#previewName"),
		PreviewHandle:    document.Call("querySelector", "#previewHandle"),
		PreviewBio:       document.Call("querySelector", "#previewBio"),
		PreviewTitle:     document.Call("querySelector", "#previewTitle"),
		Avatar:           document.Call("querySelector", "#avatar"),
		EmbedStack:       document.Call("querySelector", "#embedStack"),
		ThemeNote:        document.Call("querySelector", "#themeNote"),
		StatusText:       document.Call("querySelector", "#statusText"),
		PlaylistTemplate: document.Call("querySelector", "#playlistItemTemplate"),
		EmbedTemplate:    document.Call("querySelector", "#embedTemplate"),
		PublishButton:    document.Call("querySelector", "#publishButton"),
		HandleHint:       document.Call("querySelector", "#handleHint"),
		SaveStatus:       document.Call("querySelector", "#saveStatus"),
		CreatedPanel:     document.Call("querySelector", "#createdPanel"),
		PublicURLValue:   document.Call("querySelector", "#publicUrlValue"),
		EditURLValue:     document.Call("querySelector", "#editUrlValue"),
		CopyPublicURL:    document.Call("querySelector", "#copyPublicUrl"),
		CopyEditURL:      document.Call("querySelector", "#copyEditUrl"),
		CreateAnother:    document.Call("querySelector", "#createAnother"),
		DiscoverCreate:   document.Call("querySelector", "#discoverCreate"),
	}
	return app
}

func (a *App) bindEvents() {
	a.on(a.elements.PageForm, "input", func(event js.Value) {
		target := event.Get("target")
		if !matches(target, "input, textarea") {
			return
		}
		a.syncFormToState()
		a.persistAndRender()
	})

	a.on(a.elements.PageTitle, "input", func(js.Value) {
		a.state.Playlist.Title = strings.TrimSpace(a.elements.PageTitle.Get("value").String())
		a.persistAndRender()
	})

	a.on(a.elements.AddLink, "click", func(js.Value) {
		a.addPlaylistFromInput()
	})

	a.on(a.elements.AddSpotify, "click", func(js.Value) {
		a.addSpotifyPreview()
	})

	a.on(a.elements.EmbedStack, "click", func(event js.Value) {
		a.handleWebviewToggle(event)
	})

	a.on(a.elements.CloseWebview, "click", func(js.Value) {
		a.closePlaylistWebview()
	})

	a.on(a.elements.CloseInline, "click", func(js.Value) {
		a.closePlaylistWebview()
	})

	a.bindResponsivePlayerChange()

	a.onWithOptions(a.window, "wheel", false, func(event js.Value) {
		a.handleViewWheel(event)
	})

	a.onWithOptions(a.window, "touchstart", true, func(event js.Value) {
		a.handleViewTouchStart(event)
	})

	a.onWithOptions(a.window, "touchmove", false, func(event js.Value) {
		a.handleViewTouchMove(event)
	})

	a.on(a.window, "keydown", func(event js.Value) {
		a.handleViewKeydown(event)
	})

	a.on(a.elements.PlaylistURL, "keydown", func(event js.Value) {
		if event.Get("key").String() == "Enter" {
			event.Call("preventDefault")
			a.addPlaylistFromInput()
		}
	})

	a.on(a.elements.PlaylistList, "click", func(event js.Value) {
		button := closest(event.Get("target"), "button[data-id]")
		if isNil(button) {
			return
		}
		id := button.Get("dataset").Get("id").String()
		if a.activeWebviewLinkID == id {
			a.activeWebviewLinkID = ""
		}
		a.state.Playlist.Links = filterLinks(a.state.Playlist.Links, id)
		a.persistAndRender()
	})

	a.on(a.elements.ThemeGrid, "click", func(event js.Value) {
		button := closest(event.Get("target"), "[data-theme-option]")
		if isNil(button) {
			return
		}
		a.state.Theme = button.Get("dataset").Get("themeOption").String()
		a.persistAndRender()
	})

	for _, button := range a.queryAll("[data-density]") {
		btn := button
		a.on(btn, "click", func(js.Value) {
			a.state.Density = btn.Get("dataset").Get("density").String()
			a.persistAndRender()
		})
	}

	a.on(a.elements.ViewOnly, "click", func(js.Value) {
		a.isViewOnly = true
		a.window.Get("history").Call("replaceState", nil, "", "#view")
		a.renderEmbeds()
		a.renderViewMode()
	})

	a.on(a.elements.ExitView, "click", func(js.Value) {
		a.isViewOnly = false
		a.activeWebviewLinkID = ""
		a.window.Get("history").Call("replaceState", nil, "", a.window.Get("location").Get("pathname").String()+a.window.Get("location").Get("search").String())
		a.renderEmbeds()
		a.renderViewMode()
	})

	a.on(a.window, "hashchange", func(js.Value) {
		// The #view hash is only a local preview toggle while authoring; public
		// (view) and created pages own their own view state.
		if a.mode != ModeCreate && a.mode != ModeEdit {
			return
		}
		a.isViewOnly = a.window.Get("location").Get("hash").String() == "#view"
		if !a.isViewOnly {
			a.activeWebviewLinkID = ""
		}
		a.renderEmbeds()
		a.renderViewMode()
	})

	a.on(a.elements.PublishButton, "click", func(js.Value) {
		a.publish()
	})

	a.on(a.elements.Handle, "input", func(js.Value) {
		if a.mode == ModeCreate {
			a.scheduleHandleCheck()
		}
	})

	a.on(a.elements.CopyPublicURL, "click", func(js.Value) {
		a.copyToClipboard(a.elements.PublicURLValue.Get("textContent").String(), a.elements.CopyPublicURL)
	})

	a.on(a.elements.CopyEditURL, "click", func(js.Value) {
		a.copyToClipboard(a.elements.EditURLValue.Get("textContent").String(), a.elements.CopyEditURL)
	})

	a.on(a.elements.CreateAnother, "click", func(js.Value) {
		a.navigate("/create")
	})

	a.on(a.elements.DiscoverCreate, "click", func(js.Value) {
		a.navigate("/create")
	})

	for _, sel := range []string{"#discoverCreateHero", "#discoverCreateBand"} {
		if btn := a.document.Call("querySelector", sel); !isNil(btn) {
			a.on(btn, "click", func(js.Value) {
				a.navigate("/create")
			})
		}
	}

	a.on(a.window, "popstate", func(js.Value) {
		a.applyRoute(a.currentRoute())
	})

	a.bindLinkInterception()

	// Persistent callbacks reused by the debounced autosave + handle check, so
	// each keystroke re-arms the same timer instead of leaking a new js.Func.
	a.saveCb = js.FuncOf(func(js.Value, []js.Value) any {
		go func() {
			status, err := a.apiPutState(a.editPath(), a.state)
			if err != nil || status != 200 {
				a.setSaveStatus("Save failed — will retry on next change")
				return
			}
			a.setSaveStatus("Saved")
		}()
		return nil
	})
	a.callbacks = append(a.callbacks, a.saveCb)

	a.handleCheckCb = js.FuncOf(func(js.Value, []js.Value) any {
		h := normalizeHandle(a.elements.Handle.Get("value").String())
		if h == "" {
			a.setHandleHint("", "")
			return nil
		}
		a.setHandleHint("Checking availability…", "")
		go func() {
			available, err := a.apiHandleAvailable(h)
			if err != nil {
				a.setHandleHint("", "")
				return
			}
			if available {
				a.setHandleHint("@"+h+" is available", "is-available")
			} else {
				a.setHandleHint("@"+h+" is taken — a number will be added", "is-taken")
			}
		}()
		return nil
	})
	a.callbacks = append(a.callbacks, a.handleCheckCb)
}

// bindLinkInterception turns in-app anchors (discovery cards, the "create your
// own page" CTA, brand links back to "/") into client-side route changes. Before
// this, those plain <a href> clicks did a full document navigation: the browser
// kept the old page on screen while it re-fetched index.html and re-instantiated
// the multi-MB wasm bundle — so jumping from /@handle to /create lingered on the
// old page for a beat, then hard-cut to the new one. Routing them through
// navigate() swaps screens instantly with no reload.
//
// One delegated listener on the document covers all current and future anchors.
// It bows out for anything that should behave like a normal link: modified or
// non-primary clicks (open-in-new-tab), target=_blank / download, cross-origin
// URLs (the playlist embeds), and same-page #hash jumps. data-native is an
// explicit escape hatch to force a full navigation on a specific link.
func (a *App) bindLinkInterception() {
	a.on(a.document, "click", func(event js.Value) {
		if event.Get("defaultPrevented").Bool() || event.Get("button").Int() != 0 {
			return
		}
		if event.Get("metaKey").Bool() || event.Get("ctrlKey").Bool() ||
			event.Get("shiftKey").Bool() || event.Get("altKey").Bool() {
			return
		}
		anchor := closest(event.Get("target"), "a[href]")
		if isNil(anchor) {
			return
		}
		if t := anchor.Get("target").String(); t != "" && t != "_self" {
			return
		}
		if anchor.Call("hasAttribute", "download").Bool() ||
			anchor.Call("hasAttribute", "data-native").Bool() {
			return
		}
		// anchor.href is already resolved to an absolute URL by the DOM.
		url := a.window.Get("URL").New(anchor.Get("href").String())
		if url.Get("origin").String() != a.window.Get("location").Get("origin").String() {
			return
		}
		path := url.Get("pathname").String()
		if !isAppRoute(path) {
			return
		}
		// Same path + a hash → let the browser scroll to the anchor.
		if path == a.window.Get("location").Get("pathname").String() && url.Get("hash").String() != "" {
			return
		}
		event.Call("preventDefault")
		a.navigate(path + url.Get("search").String())
	})
}

func (a *App) bindResponsivePlayerChange() {
	if a.responsivePlayer.Get("addEventListener").Type() == js.TypeFunction {
		a.on(a.responsivePlayer, "change", func(js.Value) {
			a.renderViewMode()
		})
		return
	}

	callback := js.FuncOf(func(this js.Value, args []js.Value) any {
		a.renderViewMode()
		return nil
	})
	a.callbacks = append(a.callbacks, callback)
	a.responsivePlayer.Call("addListener", callback)
}

func (a *App) on(target js.Value, eventName string, handler func(js.Value)) {
	callback := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			handler(args[0])
		} else {
			handler(js.Undefined())
		}
		return nil
	})
	a.callbacks = append(a.callbacks, callback)
	target.Call("addEventListener", eventName, callback)
}

func (a *App) onWithOptions(target js.Value, eventName string, passive bool, handler func(js.Value)) {
	callback := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			handler(args[0])
		}
		return nil
	})
	options := js.Global().Get("Object").New()
	options.Set("passive", passive)
	a.callbacks = append(a.callbacks, callback)
	target.Call("addEventListener", eventName, callback, options)
}

func (a *App) hydrateForm() {
	a.elements.DisplayName.Set("value", a.state.User.DisplayName)
	a.elements.Handle.Set("value", a.state.User.Handle)
	a.elements.Bio.Set("value", a.state.User.Bio)
	a.elements.PageTitle.Set("value", a.state.Playlist.Title)
}

func (a *App) render() {
	a.renderModeChrome()
	a.elements.AppShell.Get("dataset").Set("theme", a.state.Theme)
	a.elements.AppShell.Get("dataset").Set("density", a.state.Density)
	a.elements.PublicPage.Get("dataset").Set("theme", a.state.Theme)

	setText(a.elements.PreviewName, fallback(a.state.User.DisplayName, "Your Name"))
	setText(a.elements.PreviewHandle, "@"+fallback(a.state.User.Handle, "yourhandle"))
	setText(a.elements.PreviewBio, fallback(a.state.User.Bio, "Add a short note about the playlists you share."))
	setText(a.elements.PreviewTitle, fallback(a.state.Playlist.Title, "Name of the Playlist"))
	// The profile avatar is a vinyl record pressed for this user: initials in the
	// cream center, @handle on the accent band — the same record motif as the
	// discovery covers.
	a.elements.Avatar.Get("classList").Call("add", "avatar-vinyl")
	a.elements.Avatar.Call("replaceChildren", a.vinylNode(
		fallback(a.state.User.Handle, a.state.User.DisplayName),
		getInitials(fallback(a.state.User.DisplayName, a.state.User.Handle)),
		"",
		"", // @handle already sits beside the avatar — no band text needed
	))
	setText(a.elements.ThemeNote, themeCopy[a.state.Theme])

	a.renderThemeControls()
	a.renderDensityControls()
	a.renderPlaylistList()
	a.renderEmbeds()
	a.renderViewMode()
}

func (a *App) renderThemeControls() {
	for _, button := range a.queryAll("[data-theme-option]") {
		active := button.Get("dataset").Get("themeOption").String() == a.state.Theme
		toggleClass(button, "is-active", active)
		button.Call("setAttribute", "aria-checked", fmt.Sprint(active))
	}
}

func (a *App) renderDensityControls() {
	for _, button := range a.queryAll("[data-density]") {
		toggleClass(button, "is-active", button.Get("dataset").Get("density").String() == a.state.Density)
	}
}

func (a *App) renderPlaylistList() {
	a.elements.PlaylistList.Call("replaceChildren")

	for _, link := range a.state.Playlist.Links {
		item := a.el("article")
		item.Get("classList").Call("add", "playlist-item")

		body := a.el("div")
		title := a.el("strong")
		setText(title, getPlaylistName(link))
		meta := a.el("span")
		setText(meta, link.Platform+" · "+link.URL)
		body.Call("append", title, meta)

		button := a.el("button")
		button.Set("type", "button")
		button.Call("setAttribute", "aria-label", "Remove playlist link")
		button.Set("title", "Remove link")
		button.Get("dataset").Set("id", link.ID)
		setText(button, "x")

		item.Call("append", body, button)
		a.elements.PlaylistList.Call("append", item)
	}
}

type embedRenderSignature struct {
	Kind   string
	Source string
}

func (a *App) renderEmbeds() {
	if len(a.state.Playlist.Links) == 0 {
		a.elements.EmbedStack.Call("replaceChildren")
		empty := a.el("article")
		empty.Get("classList").Call("add", "music-block")
		fallbackLink := a.el("a")
		fallbackLink.Get("classList").Call("add", "fallback-link")
		fallbackLink.Set("href", "#")
		fallbackLink.Call("setAttribute", "aria-disabled", "true")
		strong := a.el("strong")
		setText(strong, "Songs will show here")
		span := a.el("span")
		setText(span, "Add a public playlist from YouTube, Spotify, Apple Music, SoundCloud, or any shareable URL.")
		fallbackLink.Call("append", strong, span)
		empty.Call("append", fallbackLink)
		a.elements.EmbedStack.Call("append", empty)
		return
	}

	// Reconcile in place: moving a block (re-appending it) re-parents its
	// <iframe>, which forces the embed to reload. So drop removed blocks first,
	// then insert each block only when it isn't already at the right spot —
	// untouched blocks keep their loaded iframes.
	nextIDs := map[string]bool{}
	for _, link := range a.state.Playlist.Links {
		nextIDs[link.ID] = true
	}
	for _, child := range childrenOf(a.elements.EmbedStack) {
		id := datasetString(child, "linkId")
		if id == "" || !nextIDs[id] {
			child.Call("remove")
		}
	}

	existingBlocks := a.existingEmbedBlocks()
	cursor := a.elements.EmbedStack.Get("firstElementChild")
	for _, link := range a.state.Playlist.Links {
		block, ok := existingBlocks[link.ID]
		if !ok {
			block = a.createEmbedBlock(link)
		}
		a.updateEmbedBlock(block, link)
		if !isNil(cursor) && block.Equal(cursor) {
			cursor = cursor.Get("nextElementSibling")
		} else {
			a.elements.EmbedStack.Call("insertBefore", block, cursor)
		}
	}
}

func (a *App) existingEmbedBlocks() map[string]js.Value {
	blocks := map[string]js.Value{}
	for _, child := range childrenOf(a.elements.EmbedStack) {
		if id := datasetString(child, "linkId"); id != "" {
			blocks[id] = child
		}
	}
	return blocks
}

func (a *App) createEmbedBlock(link Link) js.Value {
	block := a.el("article")
	block.Get("classList").Call("add", "music-block")
	block.Get("dataset").Set("linkId", link.ID)

	meta := a.el("div")
	meta.Get("classList").Call("add", "music-meta")

	pill := a.el("span")
	pill.Get("classList").Call("add", "platform-pill")

	anchor := a.el("a")
	anchor.Set("target", "_blank")
	anchor.Set("rel", "noreferrer")

	toggle := a.el("button")
	toggle.Get("classList").Call("add", "webview-toggle")
	toggle.Set("type", "button")

	target := a.el("div")
	target.Get("classList").Call("add", "embed-target")

	meta.Call("append", pill, anchor, toggle)
	block.Call("append", meta, target)
	return block
}

func (a *App) updateEmbedBlock(block js.Value, link Link) {
	block.Get("dataset").Set("linkId", link.ID)
	toggleClass(block, "spotify-block", link.Platform == "Spotify")

	pill := block.Call("querySelector", ".platform-pill")
	setText(pill, link.Platform)

	anchor := block.Call("querySelector", ".music-meta a")
	anchor.Set("href", link.URL)
	setText(anchor, getPlaylistName(link))

	toggle := block.Call("querySelector", ".webview-toggle")
	a.renderWebviewToggle(toggle, link)

	target := block.Call("querySelector", ".embed-target")
	a.renderEmbedTarget(target, link)
}

func (a *App) renderEmbedTarget(target js.Value, link Link) {
	signature := getEmbedRenderSignature(link)
	if datasetString(target, "renderKind") == signature.Kind && datasetString(target, "renderSource") == signature.Source {
		if signature.Kind == "embed" {
			iframe := target.Call("querySelector", "iframe")
			if !isNil(iframe) {
				iframe.Set("title", link.Platform+" playlist embed")
			}
		}
		return
	}

	target.Get("dataset").Set("renderKind", signature.Kind)
	target.Get("dataset").Set("renderSource", signature.Source)

	switch signature.Kind {
	case "youtube":
		target.Call("replaceChildren", a.createYoutubeCard(link))
	case "embed":
		a.mountIframe(target, a.createEmbedIframe(link, link.Platform+" playlist embed"))
	default:
		target.Call("replaceChildren", a.createFallbackLink(link))
	}
}

func getEmbedRenderSignature(link Link) embedRenderSignature {
	if isYoutubeFamily(link) {
		return embedRenderSignature{
			Kind:   "youtube",
			Source: strings.Join([]string{link.Platform, link.URL, link.EmbedURL}, "|"),
		}
	}

	if canRenderEmbed(link) {
		return embedRenderSignature{Kind: "embed", Source: link.EmbedURL}
	}

	return embedRenderSignature{Kind: "fallback", Source: link.URL}
}

func (a *App) renderWebviewToggle(button js.Value, link Link) {
	isActive := a.activeWebviewLinkID == link.ID
	canOpenInline := canRenderEmbed(link)
	action := "Open player"
	if isActive {
		action = "Close player"
	}

	button.Get("dataset").Set("linkId", link.ID)
	if canOpenInline {
		setText(button, action)
	} else {
		setText(button, "No player")
	}
	button.Set("disabled", !canOpenInline)
	toggleClass(button, "is-active", isActive)
	button.Call("setAttribute", "aria-pressed", fmt.Sprint(isActive))
	if canOpenInline {
		button.Call("setAttribute", "aria-label", action+" for "+getPlaylistName(link))
		button.Set("title", action)
	} else {
		button.Call("setAttribute", "aria-label", getPlaylistName(link)+" cannot play on this page")
		button.Set("title", getFallbackReason(link))
	}
}

func (a *App) handleWebviewToggle(event js.Value) {
	button := closest(event.Get("target"), ".webview-toggle")
	if isNil(button) {
		return
	}
	event.Call("preventDefault")

	link, ok := a.findPlaylistLink(button.Get("dataset").Get("linkId").String())
	if !ok || !canRenderEmbed(link) {
		a.setStatus("Inline playback is not available for this playlist.")
		return
	}

	if !a.isViewOnly {
		a.setStatus("Use View Page to open playlist players.")
		return
	}

	if a.activeWebviewLinkID == link.ID {
		a.activeWebviewLinkID = ""
	} else {
		a.activeWebviewLinkID = link.ID
		// Record a play only for real public visitors, not author previews.
		if a.mode == ModeView {
			go a.apiPostEvent(a.routeHandle, "link_play", link.ID, link.Platform)
		}
	}
	a.renderEmbeds()
	a.renderViewMode()
}

func (a *App) closePlaylistWebview() {
	a.activeWebviewLinkID = ""
	a.renderEmbeds()
	a.renderViewMode()
}

func (a *App) renderWebview() {
	activeLink, canShowWebview := a.getActivePlayerLink()
	canShowWebview = canShowWebview && !a.usesResponsivePlayer()

	if !canShowWebview {
		a.elements.PlaylistWebview.Set("hidden", true)
		a.elements.WebviewFrame.Call("replaceChildren")
		a.elements.WebviewFrame.Get("dataset").Set("src", "")
		return
	}

	a.elements.PlaylistWebview.Set("hidden", false)
	setText(a.elements.WebviewPlatform, activeLink.Platform)
	setText(a.elements.WebviewTitle, getPlaylistName(activeLink))

	if a.elements.WebviewFrame.Get("dataset").Get("src").String() == activeLink.EmbedURL {
		return
	}

	iframe := a.createEmbedIframe(activeLink, activeLink.Platform+" inline playlist player")
	a.elements.WebviewFrame.Get("dataset").Set("src", activeLink.EmbedURL)
	a.mountIframe(a.elements.WebviewFrame, iframe)
	a.resetSplitScroll()
}

func (a *App) renderResponsivePlayer() {
	activeLink, canShowPlayer := a.getActivePlayerLink()
	canShowPlayer = canShowPlayer && a.usesResponsivePlayer()

	if !canShowPlayer {
		a.elements.InlinePlayer.Set("hidden", true)
		a.elements.InlinePlayer.Get("classList").Call("remove", "spotify-block", "youtube-block", "apple-block")
		a.elements.InlineFrame.Call("replaceChildren")
		a.elements.InlineFrame.Get("dataset").Set("src", "")
		return
	}

	a.elements.InlinePlayer.Set("hidden", false)
	toggleClass(a.elements.InlinePlayer, "spotify-block", activeLink.Platform == "Spotify")
	toggleClass(a.elements.InlinePlayer, "youtube-block", isYoutubeFamily(activeLink))
	toggleClass(a.elements.InlinePlayer, "apple-block", activeLink.Platform == "Apple Music")
	setText(a.elements.InlinePlatform, activeLink.Platform)
	setText(a.elements.InlineTitle, getPlaylistName(activeLink))

	if a.elements.InlineFrame.Get("dataset").Get("src").String() == activeLink.EmbedURL {
		return
	}

	iframe := a.createEmbedIframe(activeLink, activeLink.Platform+" playlist player")
	a.elements.InlineFrame.Get("dataset").Set("src", activeLink.EmbedURL)
	a.mountIframe(a.elements.InlineFrame, iframe)
}

func (a *App) getActivePlayerLink() (Link, bool) {
	if !a.isViewOnly {
		return Link{}, false
	}
	link, ok := a.findPlaylistLink(a.activeWebviewLinkID)
	if !ok || !canRenderEmbed(link) {
		return Link{}, false
	}
	return link, true
}

func (a *App) usesResponsivePlayer() bool {
	return a.responsivePlayer.Get("matches").Bool()
}

func (a *App) createEmbedIframe(link Link, title string) js.Value {
	iframe := a.el("iframe")
	iframe.Set("loading", "lazy")
	iframe.Set("scrolling", "no")
	iframe.Set("allow", "autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture")
	iframe.Set("referrerPolicy", "strict-origin-when-cross-origin")
	iframe.Set("title", title)
	iframe.Set("src", link.EmbedURL)
	return iframe
}

// mountIframe inserts an embed iframe into its container and shows a loading
// spinner (CSS .is-loading) until the iframe's load event fires, with a safety
// timeout so a failed embed never spins forever.
func (a *App) mountIframe(container, iframe js.Value) {
	container.Get("classList").Call("add", "is-loading")

	var loadCb, timeoutCb js.Func
	var timeoutID js.Value
	settled := false
	settle := func() {
		if settled {
			return
		}
		settled = true
		container.Get("classList").Call("remove", "is-loading")
		a.window.Call("clearTimeout", timeoutID)
		iframe.Call("removeEventListener", "load", loadCb)
		loadCb.Release()
		timeoutCb.Release()
	}
	loadCb = js.FuncOf(func(js.Value, []js.Value) any {
		settle()
		return nil
	})
	timeoutCb = js.FuncOf(func(js.Value, []js.Value) any {
		settle()
		return nil
	})

	options := js.Global().Get("Object").New()
	options.Set("once", true)
	iframe.Call("addEventListener", "load", loadCb, options)
	timeoutID = a.window.Call("setTimeout", timeoutCb, 12000)

	container.Call("replaceChildren", iframe)
}

func (a *App) createFallbackLink(link Link) js.Value {
	fallbackLink := a.el("a")
	fallbackLink.Get("classList").Call("add", "fallback-link")
	fallbackLink.Set("href", link.URL)
	fallbackLink.Set("target", "_blank")
	fallbackLink.Set("rel", "noreferrer")
	strong := a.el("strong")
	setText(strong, "Open public playlist")
	span := a.el("span")
	setText(span, getFallbackReason(link))
	fallbackLink.Call("append", strong, span)
	return fallbackLink
}

func (a *App) createYoutubeCard(link Link) js.Value {
	card := a.el("a")
	card.Get("classList").Call("add", "platform-card", "youtube-card")
	card.Set("href", link.URL)
	card.Set("target", "_blank")
	card.Set("rel", "noreferrer")

	visual := a.el("div")
	visual.Get("classList").Call("add", "platform-visual")
	playBadge := a.el("span")
	playBadge.Get("classList").Call("add", "play-badge")
	queueLines := a.el("span")
	queueLines.Get("classList").Call("add", "queue-lines")
	queueLines.Call("append", a.el("span"), a.el("span"), a.el("span"))
	openArrow := a.el("span")
	openArrow.Get("classList").Call("add", "open-arrow")
	setText(openArrow, "↗")
	visual.Call("append", playBadge, queueLines, openArrow)

	title := a.el("strong")
	if link.Platform == "YouTube Music" {
		setText(title, "Open in YouTube Music")
	} else {
		setText(title, "Open on YouTube")
	}

	small := a.el("small")
	if link.EmbedURL != "" {
		setText(small, "Public video and playlist link")
	} else {
		setText(small, "Public music playlist link")
	}

	card.Call("append", visual, title, small)
	return card
}

func (a *App) handleViewWheel(event js.Value) {
	if !a.isViewOnly || !isNil(closest(event.Get("target"), "#exitView")) {
		return
	}
	if a.activeWebviewLinkID != "" && isNil(closest(event.Get("target"), "#publicPage")) {
		return
	}
	event.Call("preventDefault")
	a.scrollViewPlaylist(event.Get("deltaY").Float())
}

func (a *App) handleViewTouchStart(event js.Value) {
	// The public page scrolls natively (smooth momentum, and embedded player
	// iframes don't trap the gesture). Only the author preview / split player
	// needs the manual scroll redirect below.
	if a.mode == ModeView {
		return
	}
	if !a.isViewOnly || event.Get("touches").Get("length").Int() == 0 {
		return
	}
	a.lastTouchY = event.Get("touches").Index(0).Get("clientY").Float()
}

func (a *App) handleViewTouchMove(event js.Value) {
	if a.mode == ModeView {
		return
	}
	if !a.isViewOnly || event.Get("touches").Get("length").Int() == 0 || !isNil(closest(event.Get("target"), "#exitView")) {
		return
	}
	if a.activeWebviewLinkID != "" && isNil(closest(event.Get("target"), "#publicPage")) {
		return
	}
	event.Call("preventDefault")

	nextY := event.Get("touches").Index(0).Get("clientY").Float()
	a.scrollViewPlaylist(a.lastTouchY - nextY)
	a.lastTouchY = nextY
}

func (a *App) handleViewKeydown(event js.Value) {
	if !a.isViewOnly || !isNil(closest(event.Get("target"), "#exitView")) {
		return
	}
	if a.activeWebviewLinkID != "" && !isNil(closest(event.Get("target"), ".playlist-webview")) {
		return
	}

	key := event.Get("key").String()
	target := a.viewScrollTarget()
	scrollHeight := target.Get("scrollHeight").Float()
	clientHeight := target.Get("clientHeight").Float()
	scrollAmounts := map[string]float64{
		"ArrowDown": 64,
		"ArrowUp":   -64,
		"PageDown":  clientHeight * 0.85,
		"PageUp":    clientHeight * -0.85,
		" ":         clientHeight * 0.85,
		"Home":      -scrollHeight,
		"End":       scrollHeight,
	}
	deltaY, ok := scrollAmounts[key]
	if !ok {
		return
	}
	event.Call("preventDefault")
	a.scrollViewPlaylist(deltaY)
}

// viewScrollTarget returns the element that actually scrolls in the current view
// layout. On the public page (ModeView) the whole phone scrolls as one surface —
// the data-mode="view" CSS gives .phone overflow-y:auto and leaves .embed-stack
// at overflow:visible, so setting the embed stack's scrollTop is a no-op there.
// In the author preview / split player the embed list itself is the scroll box.
func (a *App) viewScrollTarget() js.Value {
	if a.mode == ModeView {
		return a.elements.PublicPage
	}
	return a.elements.EmbedStack
}

func (a *App) scrollViewPlaylist(deltaY float64) {
	target := a.viewScrollTarget()
	if isNil(target) {
		return
	}
	target.Set("scrollTop", target.Get("scrollTop").Float()+deltaY)
}

func (a *App) resetSplitScroll() {
	var callback js.Func
	callback = js.FuncOf(func(this js.Value, args []js.Value) any {
		if a.elements.AppShell.Get("classList").Call("contains", "has-webview").Bool() {
			a.elements.PhoneStage.Set("scrollTop", 0)
		}
		callback.Release()
		return nil
	})
	a.window.Call("requestAnimationFrame", callback)
}

func (a *App) addPlaylistFromInput() {
	rawName := strings.TrimSpace(a.elements.PlaylistName.Get("value").String())
	rawURL := strings.TrimSpace(a.elements.PlaylistURL.Get("value").String())
	if rawURL == "" {
		a.setStatus("Paste a public playlist link first.")
		return
	}

	parsed, ok := parsePlaylistLink(rawURL)
	if !ok {
		a.setStatus("That does not look like a valid public URL.")
		return
	}

	if rawName != "" {
		parsed.Name = rawName
	} else {
		parsed.Name = getDefaultPlaylistName(parsed)
	}

	a.state.Playlist.Links = append([]Link{parsed}, a.state.Playlist.Links...)
	a.elements.PlaylistName.Set("value", "")
	a.elements.PlaylistURL.Set("value", "")
	a.setStatus(parsed.Name + " added.")
	a.persistAndRender()
}

func (a *App) addSpotifyPreview() {
	for _, link := range a.state.Playlist.Links {
		if isSpotifyPreview(link) {
			a.setStatus("Spotify preview is already on the page.")
			return
		}
	}

	a.state.Playlist.Links = append([]Link{createSpotifyPreviewLink()}, a.state.Playlist.Links...)
	a.setStatus("Spotify preview added.")
	a.persistAndRender()
}

func (a *App) renderViewMode() {
	if !a.isViewOnly {
		a.activeWebviewLinkID = ""
	}
	if a.activeWebviewLinkID != "" {
		if _, ok := a.getActivePlayerLink(); !ok {
			a.activeWebviewLinkID = ""
		}
	}
	_, hasActivePlayer := a.getActivePlayerLink()
	toggleClass(a.elements.AppShell, "view-only", a.isViewOnly)
	toggleClass(a.elements.AppShell, "has-webview", hasActivePlayer)
	toggleClass(a.document.Get("body"), "view-only-body", a.isViewOnly)
	a.elements.ViewOnly.Call("setAttribute", "aria-pressed", fmt.Sprint(a.isViewOnly))
	a.renderResponsivePlayer()
	a.renderWebview()
}

func (a *App) persistAndRender() {
	if a.mode == ModeEdit {
		// Published page: render locally and autosave to the server (debounced).
		a.render()
		a.scheduleServerSave()
		return
	}
	a.saveState(a.state)
	a.render()
}

func (a *App) saveState(state State) {
	encoded, err := json.Marshal(state)
	if err == nil {
		a.window.Get("localStorage").Call("setItem", storageKey, string(encoded))
	}
}

func (a *App) loadState() State {
	stored := a.window.Get("localStorage").Call("getItem", storageKey)
	if stored.IsNull() || stored.IsUndefined() {
		return defaultState()
	}
	state := mergeState(defaultState(), stored.String())
	didMigrate := migrateSavedLinks(&state)
	a.restoreSpotifyPreviewOnce(&state)
	if didMigrate {
		a.saveState(state)
	}
	return state
}

func (a *App) restoreSpotifyPreviewOnce(state *State) {
	if a.window.Get("localStorage").Call("getItem", spotifyPreviewRestoredKey).String() == "true" {
		return
	}
	for _, link := range state.Playlist.Links {
		if link.Platform == "Spotify" {
			return
		}
	}

	state.Playlist.Links = append([]Link{createSpotifyPreviewLink()}, state.Playlist.Links...)
	a.window.Get("localStorage").Call("setItem", spotifyPreviewRestoredKey, "true")
	a.saveState(*state)
}

func (a *App) findPlaylistLink(id string) (Link, bool) {
	for _, link := range a.state.Playlist.Links {
		if link.ID == id {
			return link, true
		}
	}
	return Link{}, false
}

func (a *App) setStatus(message string) {
	setText(a.elements.StatusText, message)
}

func (a *App) queryAll(selector string) []js.Value {
	nodes := a.document.Call("querySelectorAll", selector)
	count := nodes.Get("length").Int()
	values := make([]js.Value, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, nodes.Index(i))
	}
	return values
}

func childrenOf(element js.Value) []js.Value {
	children := element.Get("children")
	count := children.Get("length").Int()
	values := make([]js.Value, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, children.Index(i))
	}
	return values
}

func datasetString(element js.Value, name string) string {
	value := element.Get("dataset").Get(name)
	if value.IsNull() || value.IsUndefined() {
		return ""
	}
	return value.String()
}

func (a *App) el(tag string) js.Value {
	return a.document.Call("createElement", tag)
}

func defaultState() State {
	return State{
		User: User{
			DisplayName: "Dream Listener",
			Handle:      "dreamlistener",
			Bio:         "A small public page for every playlist I want friends to hear.",
		},
		Playlist: Playlist{
			Title: "Weekend Rotation",
			Links: []Link{
				createSpotifyPreviewLink(),
				{
					ID:       randomID(),
					Name:     "Favorite videos",
					URL:      "https://www.youtube.com/playlist?list=PLMC9KNkIncKtPzgY-5rmhvj7fax8fdxoj",
					Platform: "YouTube",
					Title:    "YouTube playlist",
					EmbedURL: "https://www.youtube.com/embed/videoseries?list=PLMC9KNkIncKtPzgY-5rmhvj7fax8fdxoj",
				},
			},
		},
		Theme:   "storybook",
		Density: "focused",
	}
}

func mergeState(base State, storedJSON string) State {
	var stored State
	if err := json.Unmarshal([]byte(storedJSON), &stored); err != nil {
		return base
	}

	if stored.Theme != "" {
		base.Theme = stored.Theme
	}
	if stored.Density != "" {
		base.Density = stored.Density
	}
	base.User = stored.User
	base.Playlist.Title = stored.Playlist.Title
	if stored.Playlist.Links != nil {
		base.Playlist.Links = normalizeLinks(stored.Playlist.Links)
	}
	return base
}

func normalizeLinks(links []Link) []Link {
	normalized := make([]Link, 0, len(links))
	for _, link := range links {
		if link.Name == "" {
			link.Name = getDefaultPlaylistName(link)
		}
		if link.ID == "" {
			link.ID = randomID()
		}
		normalized = append(normalized, link)
	}
	return normalized
}

func migrateSavedLinks(state *State) bool {
	didMigrate := false
	for i, link := range state.Playlist.Links {
		migrated, changed := migrateSavedAppleMusicLink(link)
		if changed {
			state.Playlist.Links[i] = migrated
			didMigrate = true
		}
	}
	return didMigrate
}

func migrateSavedAppleMusicLink(link Link) (Link, bool) {
	parsed, ok := parsePlaylistLink(link.URL)
	if !ok || parsed.Platform != "Apple Music" {
		return link, false
	}

	oldName := link.Name
	oldTitle := link.Title
	migrated := link
	migrated.Platform = parsed.Platform
	migrated.Title = parsed.Title
	migrated.EmbedURL = parsed.EmbedURL

	if shouldReplaceSavedAppleMusicName(oldName, oldTitle) {
		migrated.Name = getDefaultPlaylistName(migrated)
	}

	return migrated, migrated != link
}

func shouldReplaceSavedAppleMusicName(name, oldTitle string) bool {
	return name == "" || name == "Apple Music link" || name == oldTitle
}

func createSpotifyPreviewLink() Link {
	return Link{
		ID:       randomID(),
		Name:     "Today Top Hits",
		URL:      "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
		Platform: "Spotify",
		Title:    "Spotify playlist",
		EmbedURL: createSpotifyEmbedURL("playlist", "37i9dQZF1DXcBWIGoYBM5M"),
	}
}

func isSpotifyPreview(link Link) bool {
	return link.Platform == "Spotify" && strings.Contains(link.URL, "37i9dQZF1DXcBWIGoYBM5M")
}

func createSpotifyEmbedURL(kind, id string) string {
	return "https://open.spotify.com/embed/" + kind + "/" + url.PathEscape(id) + "?utm_source=generator&theme=0"
}

func createAppleMusicEmbedURL(parsed *url.URL) string {
	embedURL := *parsed
	embedURL.Host = "embed.music.apple.com"
	return embedURL.String()
}

func parsePlaylistLink(rawURL string) (Link, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Link{}, false
	}

	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	pathParts := splitPath(parsed.EscapedPath())
	link := Link{
		ID:       randomID(),
		URL:      parsed.String(),
		Platform: getPlatformName(host),
		Title:    getPlatformTitle(host, pathParts),
		EmbedURL: "",
	}

	query := parsed.Query()
	if strings.Contains(host, "music.youtube.com") {
		listID := query.Get("list")
		videoID := query.Get("v")
		link.Platform = "YouTube Music"
		if listID != "" {
			link.Title = "YouTube Music playlist"
		} else {
			link.Title = "YouTube Music link"
		}
		if videoID != "" {
			link.EmbedURL = "https://www.youtube.com/embed/" + url.PathEscape(videoID)
		}
	} else if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
		listID := query.Get("list")
		videoID := query.Get("v")
		if strings.Contains(host, "youtu.be") && len(pathParts) > 0 {
			videoID = pathParts[0]
		}
		link.Platform = "YouTube"
		if listID != "" {
			link.Title = "YouTube playlist"
			link.EmbedURL = "https://www.youtube.com/embed/videoseries?list=" + url.QueryEscape(listID)
		} else {
			link.Title = "YouTube video"
			if videoID != "" {
				link.EmbedURL = "https://www.youtube.com/embed/" + url.PathEscape(videoID)
			}
		}
	}

	if strings.Contains(host, "spotify.com") && len(pathParts) >= 2 {
		kind, id := pathParts[0], pathParts[1]
		if supportedSpotifyType(kind) {
			link.Platform = "Spotify"
			link.Title = "Spotify " + kind
			link.EmbedURL = createSpotifyEmbedURL(kind, id)
		}
	}

	if strings.Contains(host, "music.apple.com") {
		link.Platform = "Apple Music"
		if strings.Contains(parsed.EscapedPath(), "/playlist/") {
			link.Title = "Apple Music playlist"
		} else {
			link.Title = "Apple Music link"
		}
		link.EmbedURL = createAppleMusicEmbedURL(parsed)
	}

	if strings.Contains(host, "soundcloud.com") {
		link.Platform = "SoundCloud"
		link.Title = "SoundCloud link"
	}

	return link, true
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	rawParts := strings.Split(trimmed, "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		if part == "" {
			continue
		}
		decoded, err := url.PathUnescape(part)
		if err != nil {
			parts = append(parts, part)
		} else {
			parts = append(parts, decoded)
		}
	}
	return parts
}

func supportedSpotifyType(kind string) bool {
	switch kind {
	case "playlist", "album", "track", "artist", "show", "episode":
		return true
	default:
		return false
	}
}

func getPlatformName(host string) string {
	switch {
	case strings.Contains(host, "music.youtube.com"):
		return "YouTube Music"
	case strings.Contains(host, "youtube") || strings.Contains(host, "youtu.be"):
		return "YouTube"
	case strings.Contains(host, "spotify"):
		return "Spotify"
	case strings.Contains(host, "apple"):
		return "Apple Music"
	case strings.Contains(host, "soundcloud"):
		return "SoundCloud"
	default:
		parts := strings.Split(host, ".")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
		return "Playlist"
	}
}

func getPlatformTitle(host string, pathParts []string) string {
	platform := getPlatformName(host)
	tail := "public link"
	if len(pathParts) > 0 {
		tail = pathParts[len(pathParts)-1]
	}
	tail = strings.ReplaceAll(tail, "-", " ")
	tail = strings.ReplaceAll(tail, "_", " ")
	return platform + " " + limitRunes(tail, 48)
}

func getPlaylistName(link Link) string {
	if link.Name != "" {
		return link.Name
	}
	return getDefaultPlaylistName(link)
}

func getDefaultPlaylistName(link Link) string {
	switch link.Platform {
	case "Spotify":
		return "Spotify playlist"
	case "YouTube Music":
		return "YouTube Music playlist"
	case "YouTube":
		return "YouTube playlist"
	default:
		if link.Title != "" {
			return link.Title
		}
		if link.Platform != "" {
			return link.Platform + " playlist"
		}
		return "Public playlist"
	}
}

func normalizeHandle(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
	var out []rune
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r == '_' || r == '.' || r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			out = append(out, r)
		}
		if len(out) == 32 {
			break
		}
	}
	return string(out)
}

func auraSeed(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(s))))
	return h.Sum32()
}

// auraStyle returns a deterministic soft "gradient aura" CSS background seeded
// entirely from a handle — two blurred color blobs over a base gradient, hues
// derived from an FNV hash. No storage and no randomness/time, so a given
// handle paints the same aura on every render and device.
func auraStyle(seed string) string {
	h := auraSeed(seed)
	h1 := int(h % 360)
	h2 := (h1 + 35 + int((h>>9)%70)) % 360   // harmonious second hue
	h3 := (h1 + 165 + int((h>>17)%70)) % 360 // contrasting accent
	ang := int((h >> 3) % 360)
	x1, y1 := 18+int((h>>5)%34), 14+int((h>>11)%34)
	x2, y2 := 52+int((h>>13)%34), 48+int((h>>19)%40)
	return fmt.Sprintf(
		"radial-gradient(60%% 60%% at %d%% %d%%, hsl(%d 85%% 64%% / .95), transparent 70%%),"+
			"radial-gradient(58%% 58%% at %d%% %d%%, hsl(%d 80%% 56%% / .9), transparent 72%%),"+
			"linear-gradient(%ddeg, hsl(%d 68%% 46%%), hsl(%d 72%% 36%%))",
		x1, y1, h3, x2, y2, h2, ang, h1, h2)
}

// vinylAccent returns the seeded color for the lower "band" of a vinyl pressing
// label (the cream top is fixed). Seeded from the same handle as the aura so each
// record's band harmonizes with the gradient behind it.
func vinylAccent(seed string) string {
	h := auraSeed(seed)
	return fmt.Sprintf("hsl(%d 76%% 47%%)", int(h%360))
}

func getInitials(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	var initials []rune
	for _, part := range parts {
		for _, r := range part {
			initials = append(initials, unicode.ToUpper(r))
			break
		}
		if len(initials) == 2 {
			break
		}
	}
	if len(initials) == 0 {
		return "D"
	}
	return string(initials)
}

func canRenderEmbed(link Link) bool {
	return link.EmbedURL != ""
}

func getFallbackReason(Link) string {
	return "This service does not provide a universal browser embed from only the public URL."
}

func isYoutubeFamily(link Link) bool {
	return link.Platform == "YouTube" || link.Platform == "YouTube Music"
}

func filterLinks(links []Link, id string) []Link {
	filtered := make([]Link, 0, len(links))
	for _, link := range links {
		if link.ID != id {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

func randomID() string {
	crypto := js.Global().Get("crypto")
	if !crypto.IsUndefined() && !crypto.IsNull() {
		randomUUID := crypto.Get("randomUUID")
		if randomUUID.Type() == js.TypeFunction {
			return crypto.Call("randomUUID").String()
		}
	}
	return fmt.Sprintf("id-%d", time.Now().UnixNano())
}

func setText(element js.Value, value string) {
	element.Set("textContent", value)
}

func toggleClass(element js.Value, className string, active bool) {
	element.Get("classList").Call("toggle", className, active)
}

func matches(element js.Value, selector string) bool {
	if isNil(element) || element.Get("matches").Type() != js.TypeFunction {
		return false
	}
	return element.Call("matches", selector).Bool()
}

func closest(element js.Value, selector string) js.Value {
	if isNil(element) || element.Get("closest").Type() != js.TypeFunction {
		return js.Null()
	}
	return element.Call("closest", selector)
}

func isNil(value js.Value) bool {
	return value.IsNull() || value.IsUndefined()
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func limitRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
