package services

import "github.com/go-chi/chi/v5"

// AdminRestRouter mounts the admin audit log route under /admin.
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
