package services

import "github.com/go-chi/chi/v5"

// PipelineRestRouter mounts the admin pipeline dashboard routes under /admin.
type PipelineRestRouter struct {
	handler *PipelineRestService
}

// NewPipelineRestRouter constructs the pipeline admin RestRouter.
func NewPipelineRestRouter(h *PipelineRestService) *PipelineRestRouter {
	return &PipelineRestRouter{handler: h}
}

// Register mounts pipeline routes onto the supplied router.
func (rr *PipelineRestRouter) Register(router chi.Router) {
	router.Get("/dashboard/pipeline", rr.handler.getPipelineStats)
	router.Post("/dashboard/pipeline/retry", rr.handler.triggerRetry)
}
