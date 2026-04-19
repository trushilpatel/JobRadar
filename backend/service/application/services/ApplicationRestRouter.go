package services

import "github.com/go-chi/chi/v5"

// RestRouter registers all application routes onto a chi.Router.
type RestRouter struct {
	handler *RestService
}

// NewApplicationRestRouter constructs a RestRouter.
func NewApplicationRestRouter(h *RestService) *RestRouter {
	return &RestRouter{handler: h}
}

// Register mounts all application routes onto r.
func (rt *RestRouter) Register(r chi.Router) {
	r.Get("/jobs/{id}/applications", rt.handler.list)
	r.Post("/jobs/{id}/applications", rt.handler.create)
	r.Patch("/jobs/{id}/applications/{app_id}/stage", rt.handler.updateStage)
	r.Patch("/jobs/{id}/applications/{app_id}/note", rt.handler.updateNote)
	r.Get("/applications", rt.handler.listAll)
	r.Patch("/applications/{id}/stage", rt.handler.updateStageByID)
}
