package services

import "github.com/go-chi/chi/v5"

// RestRouter mounts the job routes onto a chi.Router. The router uses
// RELATIVE paths — the parent (apps/api) is responsible for mounting
// this group under /api so middleware (RequireIdentity + RateLimit) is
// applied uniformly.
type RestRouter struct {
	handler *RestService
}

// NewRestRouter constructs a RestRouter.
func NewRestRouter(h *RestService) *RestRouter {
	return &RestRouter{handler: h}
}

// Register mounts every job route under r. The parent router is expected
// to have applied auth + rate-limit middleware before calling this.
func (r *RestRouter) Register(router chi.Router) {
	router.Post("/jobs/dismiss/bulk", r.handler.bulkDismiss)
	router.Get("/jobs", r.handler.list)
	router.Get("/jobs/companies", r.handler.companies)
	router.Get("/jobs/counts", r.handler.counts)
	router.Get("/jobs/{id}", r.handler.get)
	router.Delete("/jobs/{id}", r.handler.dismiss)
	router.Patch("/jobs/{id}/save", r.handler.setSaved)
	router.Patch("/jobs/{id}/status", r.handler.setStatus)
	router.Patch("/jobs/{id}/notes", r.handler.setNotes)
}
