package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
)

// Under GOOS=js/GOARCH=wasm, net/http transparently uses the browser Fetch API,
// so we reuse the same State structs + json tags with no js.Value plumbing.
// Every call below blocks its goroutine, so callers run them inside `go func`.

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

// apiGetState fetches a {handle, state} envelope. Returns (state, httpStatus, error).
func (a *App) apiGetState(path string) (State, int, error) {
	resp, err := http.Get(a.apiURL(path))
	if err != nil {
		return State{}, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return State{}, resp.StatusCode, nil
	}
	var env stateEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return State{}, resp.StatusCode, err
	}
	return env.State, resp.StatusCode, nil
}

func (a *App) apiPostPublish(st State, handle string) (publishResult, int, error) {
	body, err := json.Marshal(map[string]any{"handle": handle, "state": st})
	if err != nil {
		return publishResult{}, 0, err
	}
	resp, err := http.Post(a.apiURL("/api/playlists"), "application/json", bytes.NewReader(body))
	if err != nil {
		return publishResult{}, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return publishResult{}, resp.StatusCode, nil
	}
	var res publishResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return publishResult{}, resp.StatusCode, err
	}
	return res, resp.StatusCode, nil
}

func (a *App) apiPutState(path string, st State) (int, error) {
	body, err := json.Marshal(map[string]any{"state": st})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPut, a.apiURL(path), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func (a *App) apiHandleAvailable(handle string) (bool, error) {
	resp, err := http.Get(a.apiURL("/api/handles/" + url.PathEscape(handle) + "/available"))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var r struct {
		Available bool `json:"available"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
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
	resp, err := http.Get(a.apiURL("/api/discover?section=" + url.QueryEscape(section)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Items []discoverItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Items, nil
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
	resp, err := http.Post(a.apiURL("/api/events"), "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	resp.Body.Close()
}
