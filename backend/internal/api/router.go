package api

import (
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/trushilpatel/jobradar/internal/api/handler"
	"github.com/trushilpatel/jobradar/internal/api/middleware"
)

func NewRouter(
	jobs *handler.JobHandler,
	apps *handler.ApplicationHandler,
	companies *handler.CompanyHandler,
	dashboard *handler.DashboardHandler,
) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware — runs on every request
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer) // catches panics, returns 500 instead of crashing

	r.Route("/jobs", func(r chi.Router) {
		r.Get("/", jobs.List)

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", jobs.Get)
			r.Delete("/", jobs.Dismiss)
			r.Patch("/save", jobs.SetSaved)
			r.Patch("/status", jobs.SetStatus)
			r.Patch("/notes", jobs.SetNotes)

			r.Get("/applications", apps.List)
			r.Post("/applications", apps.Create)

			r.Route("/applications/{app_id}", func(r chi.Router) {
				r.Patch("/stage", apps.UpdateStage)
				r.Patch("/note", apps.UpdateNote)
			})
		})
	})

	r.Get("/applications", apps.ListAll)

	r.Route("/companies", func(r chi.Router) {
		r.Get("/", companies.List)
		r.Post("/", companies.Create)

		r.Route("/{id}", func(r chi.Router) {
			r.Patch("/", companies.Update)
			r.Delete("/", companies.Delete)
			r.Get("/scan-stats", companies.ScanStats)
			r.Post("/scan", companies.TriggerScan)
		})
	})

	r.Route("/dashboard", func(r chi.Router) {
		r.Get("/stats", dashboard.Stats)
		r.Get("/activity", dashboard.Activity)
	})

	return r
}
