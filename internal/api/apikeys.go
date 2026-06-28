package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/raptor/internal/models"
	"github.com/t0mer/raptor/internal/store"
)

func (a *API) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	keys, err := a.store.ListAPIKeys(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}
	if keys == nil {
		keys = []*models.APIKey{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": keys})
}

// createAPIKey issues a new key. The plaintext is returned ONCE in this response
// and never again.
func (a *API) createAPIKey(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	plain, key, err := a.auth.IssueAPIKey(r.Context(), u.ID, body.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"key": plain, "api_key": key})
}

func (a *API) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "keyID")
	key, err := a.store.GetAPIKey(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (key != nil && key.UserID != u.ID && !u.IsAdmin()) {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load API key")
		return
	}
	if err := a.store.DeleteAPIKey(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete API key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
