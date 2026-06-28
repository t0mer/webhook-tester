// Package api implements Raptor's versioned management REST API under /api/v1.
// The SPA is a pure client of these endpoints.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/raptor/internal/actions"
	"github.com/t0mer/raptor/internal/auth"
	"github.com/t0mer/raptor/internal/netguard"
	"github.com/t0mer/raptor/internal/schedules"
	"github.com/t0mer/raptor/internal/sse"
	"github.com/t0mer/raptor/internal/store"
)

// ResponseForwarder delivers a CLI-supplied response to a pending captured
// request (the listen flow). Implemented by *capture.Capturer.
type ResponseForwarder interface {
	SetResponse(requestID string, status int, content string, headers map[string]string) bool
}

// Deps bundles the API handler dependencies.
type Deps struct {
	Store         *store.Store
	BaseURL       string
	Hub           *sse.Hub
	Actions       *actions.Service
	Schedules     *schedules.Runner
	Forwarder     ResponseForwarder
	Guard         *netguard.Guard
	Auth          *auth.Service
	RequireAuth   bool
	SecureCookies bool
}

// API holds dependencies for the management handlers.
type API struct {
	store         *store.Store
	baseURL       string
	hub           *sse.Hub
	actions       *actions.Service
	schedules     *schedules.Runner
	forwarder     ResponseForwarder
	guard         *netguard.Guard
	auth          *auth.Service
	requireAuth   bool
	secureCookies bool
}

// New constructs an API.
func New(d Deps) *API {
	guard := d.Guard
	if guard == nil {
		guard = netguard.New(nil, nil, false)
	}
	return &API{
		store:         d.Store,
		baseURL:       d.BaseURL,
		hub:           d.Hub,
		actions:       d.Actions,
		schedules:     d.Schedules,
		forwarder:     d.Forwarder,
		guard:         guard,
		auth:          d.Auth,
		requireAuth:   d.RequireAuth,
		secureCookies: d.SecureCookies,
	}
}

// Routes returns a chi router mounted under /api/v1.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()

	// Authenticate every request and gate when --require-auth is on.
	r.Use(a.auth.Middleware(a.requireAuth))

	r.Route("/auth", func(r chi.Router) {
		r.Get("/status", a.authStatus)
		r.Post("/bootstrap", a.bootstrap)
		r.Post("/login", a.login)
		r.Post("/logout", a.logout)
		r.Get("/me", a.me)
	})

	// Per-user API keys.
	r.Route("/account/api-keys", func(r chi.Router) {
		r.Get("/", a.listAPIKeys)
		r.Post("/", a.createAPIKey)
		r.Delete("/{keyID}", a.deleteAPIKey)
	})

	// User administration (admin only).
	r.Route("/users", func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Get("/", a.listUsers)
		r.Post("/", a.createUser)
		r.Put("/{userID}", a.updateUser)
		r.Delete("/{userID}", a.deleteUser)
	})

	r.Get("/action-types", a.listActionTypes)

	r.Route("/schedules", func(r chi.Router) {
		r.Get("/", a.listSchedules)
		r.Post("/", a.createSchedule)
		r.Route("/{scheduleID}", func(r chi.Router) {
			r.Get("/", a.getSchedule)
			r.Put("/", a.updateSchedule)
			r.Delete("/", a.deleteSchedule)
			r.Get("/runs", a.listScheduleRuns)
			r.Post("/run", a.runScheduleNow)
		})
	})

	r.Route("/groups", func(r chi.Router) {
		r.Get("/", a.listGroups)
		r.Post("/", a.createGroup)
		r.Put("/{groupID}", a.updateGroup)
		r.Delete("/{groupID}", a.deleteGroup)
	})

	r.Route("/tokens", func(r chi.Router) {
		r.Get("/", a.listTokens)
		r.Post("/", a.createToken)

		r.Route("/{tokenID}", func(r chi.Router) {
			r.Get("/", a.getToken)
			r.Put("/", a.updateToken)
			r.Delete("/", a.deleteToken)

			r.Get("/stream", a.streamRequests)

			r.Get("/requests.csv", a.exportCSV)

			r.Route("/actions", func(r chi.Router) {
				r.Get("/", a.listActions)
				r.Post("/", a.createAction)
				r.Put("/{actionID}", a.updateAction)
				r.Delete("/{actionID}", a.deleteAction)
			})
			r.Post("/test-action", a.testAction)
			r.Post("/replay", a.replayRequests)

			r.Route("/requests", func(r chi.Router) {
				r.Get("/", a.listRequests)
				r.Delete("/", a.deleteAllRequests)
				r.Get("/latest", a.latestRequest)
				r.Get("/{requestID}", a.getRequest)
				r.Get("/{requestID}/raw", a.rawRequest)
				r.Delete("/{requestID}", a.deleteRequest)
				r.Get("/{requestID}/files/{fileID}", a.downloadFile)
				r.Get("/{requestID}/action-runs", a.listActionRuns)
				r.Post("/{requestID}/execute", a.executeChain)
				r.Post("/{requestID}/response", a.setResponse)
			})
		})
	})

	return r
}

// listActionTypes returns the registered action type names (for the UI editor).
func (a *API) listActionTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": actions.KnownTypes()})
}
