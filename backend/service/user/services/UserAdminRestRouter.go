package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// AdminRestRouter mounts the admin user routes under /admin via Huma.
type AdminRestRouter struct {
	handler *AdminRestService
	zone    huma.Config
}

// NewAdminRestRouter constructs the admin RestRouter. zone is the shared
// /admin middleware-zone Huma config (see apispec.AdminZoneConfig).
func NewAdminRestRouter(h *AdminRestService, zone huma.Config) *AdminRestRouter {
	return &AdminRestRouter{handler: h, zone: zone}
}

// Register mounts the admin user management routes. The /roles and /status
// sub-routes are registered before the bare /{id} DELETE for readability,
// matching the pre-Huma registration — chi's radix-tree router already
// resolves static segments ahead of param segments regardless of order.
func (rr *AdminRestRouter) Register(router chi.Router) {
	api := humachi.New(router, rr.zone)
	huma.Register(api, huma.Operation{
		OperationID: "listUsersAdmin", Method: http.MethodGet, Path: "/users",
		Summary: "List users (admin)", Tags: []string{"users"},
	}, rr.handler.list)
	huma.Register(api, huma.Operation{
		OperationID: "getUserAdmin", Method: http.MethodGet, Path: "/users/{id}",
		Summary: "Get a user by ID (admin)", Tags: []string{"users"},
	}, rr.handler.get)
	huma.Register(api, huma.Operation{
		OperationID: "setUserRoles", Method: http.MethodPatch, Path: "/users/{id}/roles",
		Summary: "Set a user's roles", Tags: []string{"users"},
	}, rr.handler.setRoles)
	huma.Register(api, huma.Operation{
		OperationID: "setUserStatus", Method: http.MethodPatch, Path: "/users/{id}/status",
		Summary: "Set a user's status", Tags: []string{"users"},
	}, rr.handler.setStatus)
	huma.Register(api, huma.Operation{
		OperationID: "deleteUserAdmin", Method: http.MethodDelete, Path: "/users/{id}",
		Summary: "Delete a user (admin)", Tags: []string{"users"},
	}, rr.handler.delete)
}
