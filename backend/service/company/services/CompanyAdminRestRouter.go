package services

import "github.com/go-chi/chi/v5"

// AdminRestRouter mounts the admin company routes onto a chi.Router. Routes
// are relative; the parent router decides where to mount them (Phase 2
// mounts under /admin per phase2-contracts.md).
type AdminRestRouter struct {
	handler *AdminRestService
}

// NewAdminRestRouter constructs the admin RestRouter.
func NewAdminRestRouter(h *AdminRestService) *AdminRestRouter {
	return &AdminRestRouter{handler: h}
}

// Register mounts the admin CRUD routes. The /active toggle is registered
// before /{id} so chi resolves it as a literal segment.
func (rr *AdminRestRouter) Register(router chi.Router) {
	router.Get("/companies", rr.handler.list)
	router.Post("/companies", rr.handler.create)
	router.Patch("/companies/active", rr.handler.setAllActive)
	router.Get("/companies/{id}", rr.handler.get)
	router.Patch("/companies/{id}", rr.handler.updatePublic)
	router.Patch("/companies/{id}/scan-settings", rr.handler.updateScanSettings)
	router.Delete("/companies/{id}", rr.handler.delete)
}
