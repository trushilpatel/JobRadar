package services

import "github.com/go-chi/chi/v5"

// RestRouter mounts the public company routes onto a chi.Router. Routes are
// relative; the parent router decides where to mount them (Phase 2 mounts
// under /api per phase2-contracts.md).
type RestRouter struct {
	handler *RestService
}

// NewRestRouter constructs the public RestRouter.
func NewRestRouter(h *RestService) *RestRouter {
	return &RestRouter{handler: h}
}

// Register mounts the public read routes. Mutations live on the admin
// chain (CompanyAdminRestRouter).
func (rr *RestRouter) Register(router chi.Router) {
	router.Get("/companies", rr.handler.list)
	router.Get("/companies/{id}", rr.handler.get)
}
