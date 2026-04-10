package api

import (
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/trushilpatel/jobradar/internal/api/handler"
	"github.com/trushilpatel/jobradar/internal/api/middleware"
	"github.com/trushilpatel/jobradar/internal/logger"
)

func NewRouter(
	log logger.Logger,
	jobs *handler.JobHandler,
	apps *handler.ApplicationHandler,
	companies *handler.CompanyHandler,
	dashboard *handler.DashboardHandler,
	insights *handler.InsightsHandler,
) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware — runs on every request
	r.Use(middleware.Logger(log))
	r.Use(chimiddleware.Recoverer) // catches panics, returns 500 instead of crashing

	r.Get("/jobs", jobs.List)
	r.Get("/jobs/{id}", jobs.Get)
	r.Delete("/jobs/{id}", jobs.Dismiss)
	r.Patch("/jobs/{id}/save", jobs.SetSaved)
	r.Patch("/jobs/{id}/status", jobs.SetStatus)
	r.Patch("/jobs/{id}/notes", jobs.SetNotes)
	r.Get("/jobs/{id}/applications", apps.List)
	r.Post("/jobs/{id}/applications", apps.Create)
	r.Patch("/jobs/{id}/applications/{app_id}/stage", apps.UpdateStage)
	r.Patch("/jobs/{id}/applications/{app_id}/note", apps.UpdateNote)

	r.Get("/applications", apps.ListAll)

	r.Get("/companies", companies.List)
	r.Get("/companies/queue", companies.ListQueue)
	r.Delete("/companies/queue/{entry_id}", companies.RemoveFromQueue)
	r.Post("/companies/queue/{entry_id}/move-up", companies.MoveQueueItemUp)
	r.Post("/companies/queue/{entry_id}/move-down", companies.MoveQueueItemDown)
	r.Post("/companies", companies.Create)
	r.Patch("/companies/{id}", companies.Update)
	r.Delete("/companies/{id}", companies.Delete)
	r.Get("/companies/{id}/scan-stats", companies.ScanStats)
	r.Post("/companies/{id}/scan", companies.TriggerScan)

	r.Get("/dashboard/stats", dashboard.Stats)
	r.Get("/dashboard/activity", dashboard.Activity)

	r.Get("/insights/tech-stack", insights.TechStack)
	r.Get("/insights/company-activity", insights.CompanyActivity)
	r.Get("/insights/funnel", insights.Funnel)
	r.Get("/insights/location", insights.Location)
	r.Get("/insights/sponsorship", insights.Sponsorship)
	r.Get("/insights/velocity", insights.Velocity)
	r.Get("/insights/role-categories", insights.RoleCategories)

	return r
}
