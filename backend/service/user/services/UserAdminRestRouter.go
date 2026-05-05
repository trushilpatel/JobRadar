package services

import "github.com/go-chi/chi/v5"

// AdminRestRouter mounts the admin user routes under /admin.
type AdminRestRouter struct {
	handler *AdminRestService
}

// NewAdminRestRouter constructs the admin RestRouter.
func NewAdminRestRouter(h *AdminRestService) *AdminRestRouter {
	return &AdminRestRouter{handler: h}
}

// Register mounts the admin user management routes. The /roles and /status
// sub-routes are registered before the bare /{id} DELETE so chi resolves
// them as literal segments.
func (rr *AdminRestRouter) Register(router chi.Router) {
	router.Get("/users", rr.handler.list)
	router.Get("/users/{id}", rr.handler.get)
	router.Patch("/users/{id}/roles", rr.handler.setRoles)
	router.Patch("/users/{id}/status", rr.handler.setStatus)
	router.Delete("/users/{id}", rr.handler.delete)
}
