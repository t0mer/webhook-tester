package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/t0mer/raptor/internal/auth"
	"github.com/t0mer/raptor/internal/models"
	"github.com/t0mer/raptor/internal/store"
)

// tokenRequest is the create/update payload. Pointer fields distinguish
// "absent" from "zero" on update; create applies sensible defaults for nil.
type tokenRequest struct {
	Alias              *string `json:"alias"`
	DefaultStatus      *int    `json:"default_status"`
	DefaultContent     *string `json:"default_content"`
	DefaultContentType *string `json:"default_content_type"`
	Timeout            *int    `json:"timeout"`
	CORS               *bool   `json:"cors"`
	Expiry             *int    `json:"expiry"`
	Actions            *bool   `json:"actions"`
	RequestLimit       *int    `json:"request_limit"`
	Description        *string `json:"description"`
	Listen             *int    `json:"listen"`
	Redirect           *string `json:"redirect"`
	GroupID            *string `json:"group_id"`
	CloneFrom          *string `json:"clone_from"`
}

func (a *API) createToken(w http.ResponseWriter, r *http.Request) {
	var body tokenRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}

	tok := &models.Token{
		UUID:               uuid.NewString(),
		DefaultStatus:      200,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
		Premium:            true,
	}
	// Assign ownership to the creating user (empty in open mode).
	owner := ""
	if u, ok := auth.UserFromContext(r.Context()); ok {
		owner = u.ID
	}

	// Optionally clone settings from an existing token the caller can access.
	if body.CloneFrom != nil && *body.CloneFrom != "" {
		src, err := a.store.GetToken(r.Context(), *body.CloneFrom)
		if err != nil || !a.canAccessToken(r, src) {
			writeError(w, http.StatusBadRequest, "clone_from token not found")
			return
		}
		clone := *src
		clone.UUID = tok.UUID
		clone.Alias = ""
		clone.LatestRequestAt = nil
		tok = &clone
	}

	applyTokenRequest(tok, &body)
	tok.UserID = owner

	if tok.Alias != "" {
		if _, err := a.store.GetTokenByAlias(r.Context(), tok.Alias); err == nil {
			writeError(w, http.StatusConflict, "alias already in use")
			return
		}
	}

	if err := a.store.CreateToken(r.Context(), tok); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}
	writeJSON(w, http.StatusCreated, a.tokenView(tok))
}

func (a *API) listTokens(w http.ResponseWriter, r *http.Request) {
	// Non-admin authenticated users see only their own URLs; open mode and admins
	// see all.
	var (
		tokens []*models.Token
		err    error
	)
	if u, ok := auth.UserFromContext(r.Context()); ok && !u.IsAdmin() {
		tokens, err = a.store.ListTokensForUser(r.Context(), u.ID)
	} else {
		tokens, err = a.store.ListTokens(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}
	views := make([]tokenView, 0, len(tokens))
	for _, t := range tokens {
		views = append(views, a.tokenView(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": views})
}

func (a *API) getToken(w http.ResponseWriter, r *http.Request) {
	tok, ok := a.loadToken(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, a.tokenView(tok))
}

func (a *API) updateToken(w http.ResponseWriter, r *http.Request) {
	tok, ok := a.loadToken(w, r)
	if !ok {
		return
	}
	var body tokenRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	// Enforce alias uniqueness if it changes.
	if body.Alias != nil && *body.Alias != "" && *body.Alias != tok.Alias {
		if existing, err := a.store.GetTokenByAlias(r.Context(), *body.Alias); err == nil && existing.UUID != tok.UUID {
			writeError(w, http.StatusConflict, "alias already in use")
			return
		}
	}

	applyTokenRequest(tok, &body)
	if err := a.store.UpdateToken(r.Context(), tok); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update token")
		return
	}
	writeJSON(w, http.StatusOK, a.tokenView(tok))
}

func (a *API) deleteToken(w http.ResponseWriter, r *http.Request) {
	tok, ok := a.loadToken(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteToken(r.Context(), tok.UUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// loadToken resolves the {tokenID} URL param (by UUID then alias) and enforces
// ownership: a user may only access their own URLs (admins and open mode see
// all). A token the caller may not access is reported as not found.
func (a *API) loadToken(w http.ResponseWriter, r *http.Request) (*models.Token, bool) {
	id := chi.URLParam(r, "tokenID")
	tok, err := a.store.GetToken(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		tok, err = a.store.GetTokenByAlias(r.Context(), id)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "token not found")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load token")
		return nil, false
	}
	if !a.canAccessToken(r, tok) {
		writeError(w, http.StatusNotFound, "token not found")
		return nil, false
	}
	return tok, true
}

// canAccessToken reports whether the request's user may manage a token. In open
// mode (no authenticated user) all access is allowed; admins access everything;
// otherwise the user must own the token.
func (a *API) canAccessToken(r *http.Request, tok *models.Token) bool {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		return true
	}
	if u.IsAdmin() {
		return true
	}
	return tok.UserID == u.ID
}

func applyTokenRequest(tok *models.Token, body *tokenRequest) {
	if body.Alias != nil {
		tok.Alias = *body.Alias
	}
	if body.DefaultStatus != nil {
		tok.DefaultStatus = *body.DefaultStatus
	}
	if body.DefaultContent != nil {
		tok.DefaultContent = *body.DefaultContent
	}
	if body.DefaultContentType != nil {
		tok.DefaultContentType = *body.DefaultContentType
	}
	if body.Timeout != nil {
		tok.Timeout = *body.Timeout
	}
	if body.CORS != nil {
		tok.CORS = *body.CORS
	}
	if body.Expiry != nil {
		tok.Expiry = *body.Expiry
	}
	if body.Actions != nil {
		tok.Actions = *body.Actions
	}
	if body.RequestLimit != nil {
		tok.RequestLimit = *body.RequestLimit
	}
	if body.Description != nil {
		tok.Description = *body.Description
	}
	if body.Listen != nil {
		tok.Listen = *body.Listen
	}
	if body.Redirect != nil {
		tok.Redirect = *body.Redirect
	}
	if body.GroupID != nil {
		tok.GroupID = *body.GroupID
	}
}
