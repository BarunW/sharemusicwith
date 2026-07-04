package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"syscall/js"
)

// The browser provides no Go net/http transport under TinyGo (and it's heavy
// under standard Go too), so every server call goes through the Fetch API
// directly via syscall/js. We keep the same State structs + json tags and just
// marshal/unmarshal the request/response bodies ourselves.

type publishResult struct {
	Handle    string `json:"handle"`
	EditToken string `json:"editToken"`
	PublicURL string `json:"publicUrl"`
	EditURL   string `json:"editUrl"`
}

type stateEnvelope struct {
	Handle string `json:"handle"`
	State  State  `json:"state"`
}

func (a *App) apiURL(path string) string {
	return a.window.Get("location").Get("origin").String() + path
}

// doFetch performs a blocking Fetch from a goroutine and returns the HTTP status
// and raw response body. It blocks the calling goroutine on a channel until the
// request settles; every caller already runs inside `go func`, so the JS event
// loop stays free to drive the fetch/text promise callbacks that unblock it.
func (a *App) doFetch(method, url, body, contentType string) (status int, respBody string, err error) {
	type result struct {
		status int
		body   string
		err    error
	}
	ch := make(chan result, 1)

	opts := map[string]any{"method": method}
	if body != "" {
		opts["body"] = body
	}
	if contentType != "" {
		opts["headers"] = map[string]any{"Content-Type": contentType}
	}

	var onResp, onText, onErr js.Func
	cleanup := func() {
		onResp.Release()
		onText.Release()
		onErr.Release()
	}

	settled := false
	send := func(r result) {
		if settled {
			return
		}
		settled = true
		ch <- r
	}

	onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "network request failed"
		if len(args) > 0 && args[0].Truthy() {
			msg = args[0].Call("toString").String()
		}
		send(result{err: errors.New(msg)})
		return nil
	})

	var statusCode int
	onText = js.FuncOf(func(_ js.Value, args []js.Value) any {
		text := ""
		if len(args) > 0 {
			text = args[0].String()
		}
		send(result{status: statusCode, body: text})
		return nil
	})

	onResp = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resp := args[0]
		statusCode = resp.Get("status").Int()
		resp.Call("text").Call("then", onText).Call("catch", onErr)
		return nil
	})

	a.window.Call("fetch", url, opts).Call("then", onResp).Call("catch", onErr)

	r := <-ch
	cleanup()
	return r.status, r.body, r.err
}

// apiGetState fetches a {handle, state} envelope. Returns (state, httpStatus, error).
func (a *App) apiGetState(path string) (State, int, error) {
	status, body, err := a.doFetch("GET", a.apiURL(path), "", "")
	if err != nil {
		return State{}, 0, err
	}
	if status != 200 {
		return State{}, status, nil
	}
	var env stateEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return State{}, status, err
	}
	return env.State, status, nil
}

func (a *App) apiPostPublish(st State, handle string) (publishResult, int, error) {
	body, err := json.Marshal(map[string]any{"handle": handle, "state": st})
	if err != nil {
		return publishResult{}, 0, err
	}
	status, respBody, err := a.doFetch("POST", a.apiURL("/api/playlists"), string(body), "application/json")
	if err != nil {
		return publishResult{}, 0, err
	}
	if status != 200 && status != 201 {
		return publishResult{}, status, nil
	}
	var res publishResult
	if err := json.Unmarshal([]byte(respBody), &res); err != nil {
		return publishResult{}, status, err
	}
	return res, status, nil
}

func (a *App) apiPutState(path string, st State) (int, error) {
	body, err := json.Marshal(map[string]any{"state": st})
	if err != nil {
		return 0, err
	}
	status, _, err := a.doFetch("PUT", a.apiURL(path), string(body), "application/json")
	if err != nil {
		return 0, err
	}
	return status, nil
}

func (a *App) apiHandleAvailable(handle string) (bool, error) {
	_, body, err := a.doFetch("GET", a.apiURL("/api/handles/"+url.PathEscape(handle)+"/available"), "", "")
	if err != nil {
		return false, err
	}
	var r struct {
		Available bool `json:"available"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return false, err
	}
	return r.Available, nil
}

// discoverItem is one card in a discovery feed (mirrors the server response).
type discoverItem struct {
	Handle       string `json:"handle"`
	Title        string `json:"title"`
	DisplayName  string `json:"displayName"`
	Platform     string `json:"platform"`
	LinkCount    int    `json:"linkCount"`
	UniqueViews  int64  `json:"uniqueViews"`
	EngagedPlays int64  `json:"engagedPlays"`
	Views24h     int64  `json:"views24h"`
	Plays24h     int64  `json:"plays24h"`
}

// apiGetDiscover fetches one ranked section of the discovery feed.
func (a *App) apiGetDiscover(section string) ([]discoverItem, error) {
	status, body, err := a.doFetch("GET", a.apiURL("/api/discover?section="+url.QueryEscape(section)), "", "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, errors.New("discover request failed")
	}
	var out struct {
		Items []discoverItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// ytTrack / ytPlaylist mirror the server's /api/youtube/... track listing.
type ytTrack struct {
	VideoID  string `json:"videoId"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Position int64  `json:"position"`
}

type ytPlaylist struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Channel   string    `json:"channel"`
	Tracks    []ytTrack `json:"tracks"`
	Total     int       `json:"total"`
	Truncated bool      `json:"truncated"`
}

// apiGetYoutubeTracks lists a YouTube/YouTube Music playlist's songs via the
// server-side proxy (the API key never reaches the browser). ok=false on any
// failure — including the feature being off server-side — so callers just
// hide the tracklist instead of surfacing an error.
func (a *App) apiGetYoutubeTracks(listID string) (ytPlaylist, bool) {
	status, body, err := a.doFetch("GET", a.apiURL("/api/youtube/playlists/"+url.PathEscape(listID)+"/tracks"), "", "")
	if err != nil || status != 200 {
		return ytPlaylist{}, false
	}
	var pl ytPlaylist
	if err := json.Unmarshal([]byte(body), &pl); err != nil {
		return ytPlaylist{}, false
	}
	return pl, true
}

// apiPostEvent records a metric event. Fire-and-forget; callers wrap it in `go`.
func (a *App) apiPostEvent(handle, eventType, linkID, platform string) {
	body, err := json.Marshal(map[string]string{
		"handle":    handle,
		"eventType": eventType,
		"linkId":    linkID,
		"platform":  platform,
	})
	if err != nil {
		return
	}
	a.doFetch("POST", a.apiURL("/api/events"), string(body), "application/json")
}
