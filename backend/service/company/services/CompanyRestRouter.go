package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// RestRouter mounts the public company routes under /api via Huma. Mounting
// through Huma (rather than raw chi) gives the operations correct OpenAPI
// documentation for free — see
// docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Slice 1 (pilot).
type RestRouter struct {
	handler *RestService
	zone    huma.Config
}

// NewRestRouter constructs the public RestRouter. zone is the shared /api
// middleware-zone Huma config (see apihuma.NewZoneConfig) — every /api-zone
// slice registers its operations onto the same zone so they land in one
// combined OpenAPI document.
func NewRestRouter(h *RestService, zone huma.Config) *RestRouter {
	return &RestRouter{handler: h, zone: zone}
}

// Register mounts the public read routes. Mutations live on the admin
// chain (CompanyAdminRestRouter).
func (rr *RestRouter) Register(router chi.Router) {
	api := humachi.New(router, rr.zone)
	huma.Register(api, huma.Operation{
		OperationID: "listCompaniesPublic",
		Method:      http.MethodGet,
		Path:        "/companies",
		Summary:     "List companies",
		Tags:        []string{"companies"},
	}, rr.handler.list)
	huma.Register(api, huma.Operation{
		OperationID: "getCompanyPublic",
		Method:      http.MethodGet,
		Path:        "/companies/{id}",
		Summary:     "Get a company by ID",
		Tags:        []string{"companies"},
	}, rr.handler.get)
}
