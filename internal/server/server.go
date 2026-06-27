// Package server wires Raptor's HTTP surface: the management API under
// /api/v1, the public capture catch-all, the embedded SPA, and Swagger docs.
package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	v5emb "github.com/swaggest/swgui/v5emb"

	"github.com/t0mer/raptor/internal/actions"
	"github.com/t0mer/raptor/internal/api"
	"github.com/t0mer/raptor/internal/capture"
	"github.com/t0mer/raptor/internal/config"
	"github.com/t0mer/raptor/internal/metrics"
	"github.com/t0mer/raptor/internal/schedules"
	"github.com/t0mer/raptor/internal/sse"
	"github.com/t0mer/raptor/internal/store"
	"github.com/t0mer/raptor/internal/version"
)

// Server holds the HTTP dependencies and the built router.
type Server struct {
	cfg       config.Config
	store     *store.Store
	capturer  *capture.Capturer
	hub       *sse.Hub
	actions   *actions.Service
	schedules *schedules.Runner
	router    chi.Router
}

// New builds a Server and its router.
func New(cfg config.Config, st *store.Store, capturer *capture.Capturer, hub *sse.Hub, actionsSvc *actions.Service, runner *schedules.Runner) *Server {
	s := &Server{cfg: cfg, store: st, capturer: capturer, hub: hub, actions: actionsSvc, schedules: runner}
	s.router = s.buildRouter()
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(slogLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", s.health)
	r.Handle("/metrics", metrics.Handler())

	// Management API (versioned).
	r.Mount("/api/v1", api.New(s.store, s.cfg.BaseURL, s.hub, s.actions, s.schedules, s.capturer).Routes())

	// API docs (spec-first source of truth) with an embedded Swagger UI — no
	// external CDN dependency, so docs work fully offline.
	r.Get("/api/openapi.yaml", serveSpec)
	r.Mount("/api/docs", v5emb.New("Raptor API", "/api/openapi.yaml", "/api/docs"))

	// Public capture + embedded SPA fallback (must be last; it is the catch-all).
	r.HandleFunc("/*", s.handleCaptureOrSPA)

	return r
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","version":"` + version.Version + `"}`))
}
