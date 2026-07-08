package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// RestRouter registers all application routes via Huma.
type RestRouter struct {
	handler *RestService
	zone    huma.Config
}

// NewRestRouter constructs a RestRouter. zone is the shared /api
// middleware-zone Huma config (see apihuma.NewZoneConfig / apispec.APIZoneConfig).
func NewRestRouter(h *RestService, zone huma.Config) *RestRouter {
	return &RestRouter{handler: h, zone: zone}
}

// Register mounts all application routes onto r.
func (rt *RestRouter) Register(r chi.Router) {
	api := humachi.New(r, rt.zone)

	huma.Register(api, huma.Operation{
		OperationID: "listApplicationsByJob", Method: http.MethodGet, Path: "/jobs/{id}/applications",
		Summary: "List applications for a job", Tags: []string{"applications"},
	}, rt.handler.list)
	huma.Register(api, huma.Operation{
		OperationID: "createApplication", Method: http.MethodPost, Path: "/jobs/{id}/applications",
		Summary: "Create an application for a job", Tags: []string{"applications"},
		DefaultStatus: http.StatusCreated,
	}, rt.handler.create)
	huma.Register(api, huma.Operation{
		OperationID: "updateApplicationStage", Method: http.MethodPatch, Path: "/jobs/{id}/applications/{app_id}/stage",
		Summary: "Update an application's stage", Tags: []string{"applications"},
	}, rt.handler.updateStage)
	huma.Register(api, huma.Operation{
		OperationID: "updateApplicationNote", Method: http.MethodPatch, Path: "/jobs/{id}/applications/{app_id}/note",
		Summary: "Update an application's note", Tags: []string{"applications"},
	}, rt.handler.updateNote)
	huma.Register(api, huma.Operation{
		OperationID: "listAllApplications", Method: http.MethodGet, Path: "/applications",
		Summary: "List all applications for the current user", Tags: []string{"applications"},
	}, rt.handler.listAll)
	huma.Register(api, huma.Operation{
		OperationID: "updateApplicationStageByID", Method: http.MethodPatch, Path: "/applications/{id}/stage",
		Summary: "Update an application's stage by application ID", Tags: []string{"applications"},
	}, rt.handler.updateStageByID)
}
