package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gorm.io/gorm"
)

// NewRouter builds the public API router. Middleware chain:
// otelhttp → Logger → Metrics → Recoverer → SecurityHeaders → BodyLimit →
// Auth → (per subrouter: RequireIdentity + RateLimit).
//
// apiGroups are mounted under /api with RequireIdentity + RateLimit applied.
// rootGroups (e.g. /admin/*) are mounted at root with their own middleware.
// Operational endpoints (/metrics, /health, /ready) live on a separate
// listener — see NewOpsRouter.
func NewRouter(
	log logger.Logger,
	cfg *config.Config,
	authMiddleware func(http.Handler) http.Handler,
	identityMW func(http.Handler) http.Handler,
	rateLimitMW func(http.Handler) http.Handler,
	apiGroups []handler.RouteGroup,
	rootGroups []handler.RouteGroup,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(otelhttp.NewMiddleware("jobradar-api"))
	r.Use(middleware.Logger(log))
	r.Use(middleware.Metrics())
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.BodyLimit(cfg.API.MaxBodyBytes))
	r.Use(authMiddleware)

	r.Route("/api", func(api chi.Router) {
		if identityMW != nil {
			api.Use(identityMW)
		}
		if rateLimitMW != nil {
			api.Use(rateLimitMW)
		}
		for _, g := range apiGroups {
			g.Register(api)
		}
	})

	for _, g := range rootGroups {
		g.Register(r)
	}

	return r
}

// NewOpsRouter builds the operational router bound to the metrics listener.
// It exposes /metrics (Prometheus), /health (liveness), and /ready
// (DB ping). Kept separate from the public API so Prometheus scrapes never
// traverse the auth or rate-limit middleware stacks.
func NewOpsRouter(database *gorm.DB) *chi.Mux {
	r := chi.NewRouter()
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := db.PoolReady(database); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return r
}
