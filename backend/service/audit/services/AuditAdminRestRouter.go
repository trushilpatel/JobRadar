package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// AdminRestRouter mounts the admin audit log route under /admin via Huma.
type AdminRestRouter struct {
	handler *AdminRestService
	zone    huma.Config
}

// NewAdminRestRouter constructs the admin audit RestRouter. zone is the
// shared /admin middleware-zone Huma config (see apispec.AdminZoneConfig).
func NewAdminRestRouter(h *AdminRestService, zone huma.Config) *AdminRestRouter {
	return &AdminRestRouter{handler: h, zone: zone}
}

// Register mounts GET /audit-log onto the supplied router.
func (rr *AdminRestRouter) Register(router chi.Router) {
	api := humachi.New(router, rr.zone)
	huma.Register(api, huma.Operation{
		OperationID: "listAuditLog", Method: http.MethodGet, Path: "/audit-log",
		Summary: "List admin audit log entries", Tags: []string{"audit"},
	}, rr.handler.list)
}
