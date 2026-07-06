package services

import "github.com/go-chi/chi/v5"

// RestRouter registers the apply-queue routes. Mounted under /api by
// apps/api/router.go, which already applies RequireIdentity + RateLimit to
// every /api/* route — no admin gating needed for this user-scoped feature.
type RestRouter struct{ handler *RestService }

// NewRestRouter constructs a RestRouter.
func NewRestRouter(h *RestService) *RestRouter { return &RestRouter{handler: h} }

// Register mounts the apply-queue routes onto r.
func (rr *RestRouter) Register(r chi.Router) {
	r.Get("/apply-queue", rr.handler.list)
	r.Put("/apply-queue/{jobId}/resume", rr.handler.setResume)
}
