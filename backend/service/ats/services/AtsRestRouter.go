package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// AtsRestRouter mounts the /ats routes under /admin via Huma. The middleware
// chain that gates these routes is applied by the caller passing a chi.Router
// subtree that already has those middleware in place (see
// adminRouteGroup.Register in apps/api/wire_multiuser.go) — the router
// itself only knows route paths and verbs, registered relative to whatever
// subrouter it's given (see docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
// Slice 8).
type AtsRestRouter struct {
	handler *AtsRestService
	zone    huma.Config
}

// NewRestRouter constructs an AtsRestRouter. zone is the shared /admin
// middleware-zone Huma config (see apispec.AdminZoneConfig) — every
// /admin-zone slice registers its operations onto the same zone so they
// land in one combined OpenAPI document.
func NewRestRouter(h *AtsRestService, zone huma.Config) *AtsRestRouter {
	return &AtsRestRouter{handler: h, zone: zone}
}

// Register satisfies handler.RouteGroup. Mounts /ats routes; the caller is
// responsible for placing this group under the correct subrouter.
func (rt *AtsRestRouter) Register(r chi.Router) {
	api := humachi.New(r, rt.zone)
	huma.Register(api, huma.Operation{
		OperationID: "listAts",
		Method:      http.MethodGet,
		Path:        "/ats",
		Summary:     "List ATS adapters",
		Tags:        []string{"ats"},
	}, rt.handler.list)
	huma.Register(api, huma.Operation{
		OperationID: "getAts",
		Method:      http.MethodGet,
		Path:        "/ats/{name}",
		Summary:     "Get an ATS adapter by name",
		Tags:        []string{"ats"},
	}, rt.handler.get)
	huma.Register(api, huma.Operation{
		OperationID: "updateAts",
		Method:      http.MethodPatch,
		Path:        "/ats/{name}",
		Summary:     "Update an ATS adapter",
		Tags:        []string{"ats"},
	}, rt.handler.update)
}
