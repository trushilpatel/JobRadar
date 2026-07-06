package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// PipelineRestRouter mounts the /pipeline routes under /admin via Huma. The
// middleware chain that gates these routes is applied by the caller passing
// a chi.Router subtree that already has those middleware in place (the
// /admin group) — the router itself only knows route paths and verbs,
// registered relative to whatever subrouter it's given (see
// docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Slice 9).
type PipelineRestRouter struct {
	handler *PipelineRestService
	zone    huma.Config
}

// NewRestRouter constructs a PipelineRestRouter. zone is the shared /admin
// middleware-zone Huma config (see apispec.AdminZoneConfig).
func NewRestRouter(h *PipelineRestService, zone huma.Config) *PipelineRestRouter {
	return &PipelineRestRouter{handler: h, zone: zone}
}

// Register satisfies handler.RouteGroup. Mounts /pipeline routes; the caller
// is responsible for placing this group under the correct subrouter.
func (rt *PipelineRestRouter) Register(r chi.Router) {
	api := humachi.New(r, rt.zone)
	huma.Register(api, huma.Operation{
		OperationID: "getPipelineStats",
		Method:      http.MethodGet,
		Path:        "/pipeline/stats",
		Summary:     "Get pipeline health stats",
		Tags:        []string{"pipeline"},
	}, rt.handler.getStats)
	huma.Register(api, huma.Operation{
		OperationID: "triggerPipelineRetry",
		Method:      http.MethodPost,
		Path:        "/pipeline/retry",
		Summary:     "Trigger a retry redelivery nudge",
		Tags:        []string{"pipeline"},
	}, rt.handler.triggerRetry)
}
