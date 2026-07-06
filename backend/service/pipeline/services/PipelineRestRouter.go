package services

import "github.com/go-chi/chi/v5"

// PipelineRestRouter mounts the /pipeline routes. The middleware chain that
// gates these routes is applied by the caller passing a chi.Router subtree
// that already has those middleware in place (the /admin group) — the
// router itself only knows route paths and verbs.
type PipelineRestRouter struct {
	handler *PipelineRestService
}

// NewRestRouter constructs a PipelineRestRouter.
func NewRestRouter(h *PipelineRestService) *PipelineRestRouter {
	return &PipelineRestRouter{handler: h}
}

// Register satisfies handler.RouteGroup. Mounts /pipeline routes; the caller
// is responsible for placing this group under the correct subrouter.
func (rt *PipelineRestRouter) Register(r chi.Router) {
	r.Get("/pipeline/stats", rt.handler.GetStats)
	r.Post("/pipeline/retry", rt.handler.TriggerRetry)
}
