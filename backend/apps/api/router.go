package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gorm.io/gorm"
)

// NewRouter builds the chi router, applies global middleware, mounts operational
// endpoints, then delegates all resource routes to each RouteGroup.
//
// To add a new resource: implement handler.RouteGroup and add it to App.Handlers().
// This function never needs to change.
func NewRouter(log logger.Logger, database *gorm.DB, groups []handler.RouteGroup) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware — runs on every request
	r.Use(otelhttp.NewMiddleware("jobradar-api")) // root span per request; must be first
	r.Use(middleware.Logger(log))
	r.Use(middleware.Metrics())
	r.Use(chimiddleware.Recoverer)

	// Operational endpoints — not owned by any resource handler
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

	// Register all resource route groups
	for _, g := range groups {
		g.Register(r)
	}

	return r
}
