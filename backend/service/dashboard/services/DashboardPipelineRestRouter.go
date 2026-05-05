package services

import "github.com/go-chi/chi/v5"

// PipelineRestRouter mounts the admin pipeline dashboard route. Routes are
// relative so the parent router decides the prefix (Phase 2 mounts under
// /admin per phase2-contracts.md).
type PipelineRestRouter struct {
	handler *PipelineRestService
}

// NewPipelineRestRouter constructs the pipeline admin RestRouter.
func NewPipelineRestRouter(h *PipelineRestService) *PipelineRestRouter {
	return &PipelineRestRouter{handler: h}
}

// Register mounts GET /dashboard/pipeline onto the supplied router.
func (rr *PipelineRestRouter) Register(router chi.Router) {
	router.Get("/dashboard/pipeline", rr.handler.getPipelineStats)
}
