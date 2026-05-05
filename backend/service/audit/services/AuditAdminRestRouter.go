package services

import "github.com/go-chi/chi/v5"

// AdminRestRouter mounts the admin audit log route. Routes are relative so
// the parent router decides the prefix (Phase 2 mounts under /admin per
// phase2-contracts.md).
type AdminRestRouter struct {
	handler *AdminRestService
}

// NewAdminRestRouter constructs the admin audit RestRouter.
func NewAdminRestRouter(h *AdminRestService) *AdminRestRouter {
	return &AdminRestRouter{handler: h}
}

// Register mounts GET /audit-log onto the supplied router.
func (rr *AdminRestRouter) Register(router chi.Router) {
	router.Get("/audit-log", rr.handler.list)
}
