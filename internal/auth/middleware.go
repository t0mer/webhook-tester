package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/t0mer/raptor/internal/models"
)

// SessionCookie is the cookie name holding the session id.
const SessionCookie = "raptor_session"

type ctxKey int

const userKey ctxKey = iota

// publicPaths are reachable without authentication so login and first-run
// bootstrap can work even when the API is gated.
var publicPaths = map[string]bool{
	"/api/v1/auth/login":     true,
	"/api/v1/auth/status":    true,
	"/api/v1/auth/bootstrap": true,
}

// UserFromContext returns the authenticated user, if any.
func UserFromContext(ctx context.Context) (*models.User, bool) {
	u, ok := ctx.Value(userKey).(*models.User)
	return u, ok
}

func withUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// Middleware authenticates the request (API key, session cookie, or Basic Auth)
// and, when requireAuth is on and the instance is bootstrapped, rejects
// unauthenticated access. Identified users are attached to the request context
// regardless, so handlers can personalise responses in open mode too.
func (s *Service) Middleware(requireAuth bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u := s.identify(r); u != nil {
				r = r.WithContext(withUser(r.Context(), u))
			}

			if publicPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Gate only when auth is required AND an admin already exists.
			if requireAuth && s.Bootstrapped(r.Context()) {
				if _, ok := UserFromContext(r.Context()); !ok {
					unauthorized(w)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin wraps handlers that only administrators may call.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		if !u.IsAdmin() {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"admin role required"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// identify resolves the request's user via API key, session cookie, or Basic
// Auth. Returns nil when unauthenticated.
func (s *Service) identify(r *http.Request) *models.User {
	ctx := r.Context()
	if key := strings.TrimSpace(r.Header.Get("Api-Key")); key != "" {
		if u, err := s.UserByAPIKey(ctx, key); err == nil {
			return u
		}
	}
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		if u, err := s.UserBySession(ctx, c.Value); err == nil {
			return u
		}
	}
	if email, pass, ok := r.BasicAuth(); ok {
		if u, err := s.Login(ctx, email, pass); err == nil {
			return u
		}
	}
	return nil
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"authentication required"}`))
}
