// Package api implements Raptor's versioned management REST API under /api/v1.
// The SPA is a pure client of these endpoints.
package api

import (
	"github.com/go-chi/chi/v5"

	"github.com/t0mer/raptor/internal/sse"
	"github.com/t0mer/raptor/internal/store"
)

// API holds dependencies for the management handlers.
type API struct {
	store   *store.Store
	baseURL string
	hub     *sse.Hub
}

// New constructs an API.
func New(st *store.Store, baseURL string, hub *sse.Hub) *API {
	return &API{store: st, baseURL: baseURL, hub: hub}
}

// Routes returns a chi router mounted under /api/v1.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/tokens", func(r chi.Router) {
		r.Get("/", a.listTokens)
		r.Post("/", a.createToken)

		r.Route("/{tokenID}", func(r chi.Router) {
			r.Get("/", a.getToken)
			r.Put("/", a.updateToken)
			r.Delete("/", a.deleteToken)

			r.Get("/stream", a.streamRequests)

			r.Route("/requests", func(r chi.Router) {
				r.Get("/", a.listRequests)
				r.Delete("/", a.deleteAllRequests)
				r.Get("/latest", a.latestRequest)
				r.Get("/{requestID}", a.getRequest)
				r.Get("/{requestID}/raw", a.rawRequest)
				r.Delete("/{requestID}", a.deleteRequest)
				r.Get("/{requestID}/files/{fileID}", a.downloadFile)
			})
		})
	})

	return r
}
