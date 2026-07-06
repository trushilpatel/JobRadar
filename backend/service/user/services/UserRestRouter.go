package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// RestRouter mounts the /api/me routes via Huma, directly on the router
// passed to Register — like every other /api-zone slice, it relies purely
// on the /api zone's already-applied middleware (RequireIdentity,
// RateLimit), rather than self-applying a second copy of the same two
// middlewares on a nested "/me" mount as the pre-Huma router did. That
// self-application was fully redundant (the /api zone had already run the
// identical closures moments earlier) and, more importantly, was
// incompatible with the single-prefix-per-zone assumption the Huma spec
// merge relies on (docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
// Section 3.0 item 3): a nested mount's relative paths ("/", "/config")
// would merge into the combined spec as "/api/", not "/api/me/".
type RestRouter struct {
	handler *RestService
	zone    huma.Config
}

// NewRestRouter constructs a RestRouter. zone is the shared /api
// middleware-zone Huma config (see apihuma.NewZoneConfig / apispec.APIZoneConfig).
func NewRestRouter(h *RestService, zone huma.Config) *RestRouter {
	return &RestRouter{handler: h, zone: zone}
}

// Register satisfies handler.RouteGroup.
func (rt *RestRouter) Register(r chi.Router) {
	api := humachi.New(r, rt.zone)

	huma.Register(api, huma.Operation{
		OperationID: "getMe", Method: http.MethodGet, Path: "/me",
		Summary: "Get the current user's identity bundle", Tags: []string{"me"},
	}, rt.handler.getMe)
	huma.Register(api, huma.Operation{
		OperationID: "updateMe", Method: http.MethodPatch, Path: "/me",
		Summary: "Update the current user's profile", Tags: []string{"me"},
	}, rt.handler.updateMe)
	huma.Register(api, huma.Operation{
		OperationID: "deleteMe", Method: http.MethodDelete, Path: "/me",
		Summary: "Delete the current user's account", Tags: []string{"me"},
	}, rt.handler.deleteMe)
	huma.Register(api, huma.Operation{
		OperationID: "getMyConfig", Method: http.MethodGet, Path: "/me/config",
		Summary: "Get the current user's config", Tags: []string{"me"},
	}, rt.handler.getMyConfig)
	huma.Register(api, huma.Operation{
		OperationID: "updateMyConfig", Method: http.MethodPatch, Path: "/me/config",
		Summary: "Update the current user's config", Tags: []string{"me"},
	}, rt.handler.updateMyConfig)
}
