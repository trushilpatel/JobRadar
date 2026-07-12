package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// AdminRestRouter mounts the admin company routes under /admin via Huma.
type AdminRestRouter struct {
	handler *AdminRestService
	zone    huma.Config
}

// NewAdminRestRouter constructs the admin RestRouter. zone is the shared
// /admin middleware-zone Huma config (see apispec.AdminZoneConfig).
func NewAdminRestRouter(h *AdminRestService, zone huma.Config) *AdminRestRouter {
	return &AdminRestRouter{handler: h, zone: zone}
}

// Register mounts the admin CRUD routes. The /active toggle is registered
// before /{id} so chi resolves it as a literal segment (chi's radix-tree
// router already prefers static segments over params regardless of
// registration order, but the ordering is kept for readability, matching
// the pre-Huma registration).
func (rr *AdminRestRouter) Register(router chi.Router) {
	api := humachi.New(router, rr.zone)
	huma.Register(api, huma.Operation{
		OperationID: "listCompaniesAdmin", Method: http.MethodGet, Path: "/companies",
		Summary: "List companies (admin)", Tags: []string{"companies"},
	}, rr.handler.list)
	huma.Register(api, huma.Operation{
		OperationID: "createCompanyAdmin", Method: http.MethodPost, Path: "/companies",
		Summary: "Create a company", Tags: []string{"companies"},
		DefaultStatus: http.StatusCreated,
	}, rr.handler.create)
	huma.Register(api, huma.Operation{
		OperationID: "setAllCompaniesActive", Method: http.MethodPatch, Path: "/companies/active",
		Summary: "Set active flag for all companies", Tags: []string{"companies"},
	}, rr.handler.setAllActive)
	huma.Register(api, huma.Operation{
		OperationID: "getCompanyAdmin", Method: http.MethodGet, Path: "/companies/{id}",
		Summary: "Get a company by ID (admin)", Tags: []string{"companies"},
	}, rr.handler.get)
	huma.Register(api, huma.Operation{
		OperationID: "updateCompanyPublic", Method: http.MethodPatch, Path: "/companies/{id}",
		Summary: "Update a company's public fields", Tags: []string{"companies"},
	}, rr.handler.updatePublic)
	huma.Register(api, huma.Operation{
		OperationID: "updateCompanyScanSettings", Method: http.MethodPatch, Path: "/companies/{id}/scan-settings",
		Summary: "Update a company's scan settings", Tags: []string{"companies"},
	}, rr.handler.updateScanSettings)
	huma.Register(api, huma.Operation{
		OperationID: "deleteCompanyAdmin", Method: http.MethodDelete, Path: "/companies/{id}",
		Summary: "Delete a company", Tags: []string{"companies"},
	}, rr.handler.delete)
}
