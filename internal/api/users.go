package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/raptor/internal/auth"
	"github.com/t0mer/raptor/internal/models"
	"github.com/t0mer/raptor/internal/store"
)

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	if users == nil {
		users = []*models.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": users})
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	role := body.Role
	if role != models.RoleAdmin {
		role = models.RoleUser
	}
	u, err := a.auth.CreateUser(r.Context(), body.Email, body.Password, role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userID")
	u, err := a.store.GetUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	var body struct {
		Email    *string `json:"email"`
		Password *string `json:"password"`
		Role     *string `json:"role"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if body.Email != nil {
		u.Email = *body.Email
	}
	if body.Role != nil && (*body.Role == models.RoleAdmin || *body.Role == models.RoleUser) {
		u.Role = *body.Role
	}
	if body.Password != nil {
		if len(*body.Password) < 8 {
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		hash, err := auth.HashPassword(*body.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		u.PasswordHash = hash
	}
	if err := a.store.UpdateUser(r.Context(), u); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userID")
	// Prevent an admin from deleting their own account out from under themselves.
	if cur, ok := auth.UserFromContext(r.Context()); ok && cur.ID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	if err := a.store.DeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
