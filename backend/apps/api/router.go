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

// NewRouter builds the public API router. Middleware chain is:
// otelhttp → Logger → Metrics → Recoverer → SecurityHeaders → BodyLimit →
// Auth → route groups. Operational endpoints (/metrics, /health, /ready)
// live on a separate listener — see NewOpsRouter.
//
// authMiddleware is injected so main.go can fail fast when auth setup fails
// (e.g. malformed JWKS URL); this keeps NewRouter free of fallible IO.
func NewRouter(
	log logger.Logger,
	cfg *config.Config,
	authMiddleware func(http.Handler) http.Handler,
	groups []handler.RouteGroup,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(otelhttp.NewMiddleware("jobradar-api"))
	r.Use(middleware.Logger(log))
	r.Use(middleware.Metrics())
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.BodyLimit(cfg.API.MaxBodyBytes))
	r.Use(authMiddleware)

	for _, g := range groups {
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
		if err := db.Ping(database); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return r
}
