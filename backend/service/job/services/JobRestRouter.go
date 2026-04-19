package services

import "github.com/go-chi/chi/v5"

// RestRouter mounts all job routes onto a chi.Router.
// It owns route registration; no business logic lives here.
type RestRouter struct {
	handler *RestService
}

// NewRestRouter constructs a RestRouter.
func NewRestRouter(h *RestService) *RestRouter {
	return &RestRouter{handler: h}
}

// Register mounts all job routes onto r.
func (r *RestRouter) Register(router chi.Router) {
	router.Get("/jobs", r.handler.List)
	router.Get("/jobs/counts", r.handler.GetCounts)
	router.Get("/jobs/{id}", r.handler.Get)
	router.Delete("/jobs/{id}", r.handler.Dismiss)
	router.Patch("/jobs/{id}/save", r.handler.SetSaved)
	router.Patch("/jobs/{id}/status", r.handler.SetStatus)
	router.Patch("/jobs/{id}/notes", r.handler.SetNotes)
}
