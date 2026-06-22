package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"connect-with-playlist-wasm/internal/handle"
	"connect-with-playlist-wasm/internal/state"
	"connect-with-playlist-wasm/internal/store"
	"connect-with-playlist-wasm/internal/token"
)

type createRequest struct {
	Handle string      `json:"handle"`
	State  state.State `json:"state"`
}

type createResponse struct {
	Handle    string `json:"handle"`
	EditToken string `json:"editToken"`
	PublicURL string `json:"publicUrl"`
	EditURL   string `json:"editUrl"`
}

type stateResponse struct {
	Handle    string      `json:"handle"`
	State     state.State `json:"state"`
	ViewCount int64       `json:"viewCount,omitempty"`
}

type updateRequest struct {
	State state.State `json:"state"`
}

// POST /api/playlists
func (s *Server) createPlaylist(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Could not parse request.")
		return
	}

	h := handle.Normalize(req.Handle)
	if h == "" {
		h = handle.Normalize(req.State.User.Handle)
	}
	if err := handle.Validate(h); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_handle", "Pick a handle with letters or numbers.")
		return
	}
	if err := state.Validate(req.State); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_state", err.Error())
		return
	}

	tok, err := token.Generate()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "")
		return
	}

	finalHandle, err := s.store.CreatePlaylist(r.Context(), h, req.State, token.Hash(tok))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "Could not save the page.")
		return
	}

	writeJSON(w, http.StatusCreated, createResponse{
		Handle:    finalHandle,
		EditToken: tok,
		PublicURL: "/@" + finalHandle,
		EditURL:   "/@" + finalHandle + "/edit/" + tok,
	})
}

// GET /api/playlists/{handle}
func (s *Server) getPublicPlaylist(w http.ResponseWriter, r *http.Request) {
	h := handle.Normalize(r.PathValue("handle"))
	p, err := s.store.GetByHandle(r.Context(), h)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No page with that handle.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	// Best-effort raw-hit counter; failure here must not block the read.
	count := p.ViewCount
	if c, err := s.store.IncrementViewCount(r.Context(), h); err == nil {
		count = c
	}
	writeJSON(w, http.StatusOK, stateResponse{Handle: p.Handle, State: p.State, ViewCount: count})
}

// GET /api/playlists/{handle}/edit/{editToken}
func (s *Server) getEditPlaylist(w http.ResponseWriter, r *http.Request) {
	h := handle.Normalize(r.PathValue("handle"))
	p, err := s.store.GetForEdit(r.Context(), h, r.PathValue("editToken"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No page with that handle.")
		return
	}
	if errors.Is(err, store.ErrBadToken) {
		writeError(w, http.StatusForbidden, "forbidden", "This edit link is not valid.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	writeJSON(w, http.StatusOK, stateResponse{Handle: p.Handle, State: p.State})
}

// PUT /api/playlists/{handle}/edit/{editToken}
func (s *Server) updatePlaylist(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Could not parse request.")
		return
	}
	if err := state.Validate(req.State); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_state", err.Error())
		return
	}

	h := handle.Normalize(r.PathValue("handle"))
	err := s.store.UpdateByToken(r.Context(), h, r.PathValue("editToken"), req.State)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No page with that handle.")
		return
	}
	if errors.Is(err, store.ErrBadToken) {
		writeError(w, http.StatusForbidden, "forbidden", "This edit link is not valid.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	req.State.User.Handle = h
	writeJSON(w, http.StatusOK, stateResponse{Handle: h, State: req.State})
}

// GET /api/handles/{handle}/available
func (s *Server) checkHandleAvailable(w http.ResponseWriter, r *http.Request) {
	h := handle.Normalize(r.PathValue("handle"))
	if err := handle.Validate(h); err != nil {
		reason := "invalid"
		if errors.Is(err, handle.ErrReserved) {
			reason = "reserved"
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "reason": reason, "handle": h})
		return
	}
	exists, err := s.store.HandleExists(r.Context(), h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": !exists, "handle": h})
}
