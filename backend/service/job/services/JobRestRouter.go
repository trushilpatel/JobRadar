package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// RestRouter mounts the job routes via Huma. The router uses RELATIVE
// paths — the parent (apps/api) is responsible for mounting this group
// under /api so middleware (RequireIdentity + RateLimit) is applied
// uniformly.
type RestRouter struct {
	handler *RestService
	zone    huma.Config
}

// NewRestRouter constructs a RestRouter. zone is the shared /api
// middleware-zone Huma config (see apihuma.NewZoneConfig / apispec.APIZoneConfig).
func NewRestRouter(h *RestService, zone huma.Config) *RestRouter {
	return &RestRouter{handler: h, zone: zone}
}

// Register mounts every job route under router. The parent router is
// expected to have applied auth + rate-limit middleware before calling this.
func (rr *RestRouter) Register(router chi.Router) {
	api := humachi.New(router, rr.zone)

	huma.Register(api, huma.Operation{
		OperationID: "listJobs", Method: http.MethodGet, Path: "/jobs",
		Summary: "List jobs", Tags: []string{"jobs"},
	}, rr.handler.list)
	huma.Register(api, huma.Operation{
		OperationID: "getJobCompanies", Method: http.MethodGet, Path: "/jobs/companies",
		Summary: "List distinct job company names", Tags: []string{"jobs"},
	}, rr.handler.companies)
	huma.Register(api, huma.Operation{
		OperationID: "getJobCounts", Method: http.MethodGet, Path: "/jobs/counts",
		Summary: "Get per-status job counts for the current user", Tags: []string{"jobs"},
	}, rr.handler.counts)
	huma.Register(api, huma.Operation{
		OperationID: "getJob", Method: http.MethodGet, Path: "/jobs/{id}",
		Summary: "Get a job by ID", Tags: []string{"jobs"},
	}, rr.handler.get)
	huma.Register(api, huma.Operation{
		OperationID: "dismissJob", Method: http.MethodDelete, Path: "/jobs/{id}",
		Summary: "Dismiss a job", Tags: []string{"jobs"},
	}, rr.handler.dismiss)
	huma.Register(api, huma.Operation{
		OperationID: "setJobSaved", Method: http.MethodPatch, Path: "/jobs/{id}/save",
		Summary: "Save or unsave a job", Tags: []string{"jobs"},
	}, rr.handler.setSaved)
	huma.Register(api, huma.Operation{
		OperationID: "setJobStatus", Method: http.MethodPatch, Path: "/jobs/{id}/status",
		Summary: "Set a job's status", Tags: []string{"jobs"},
	}, rr.handler.setStatus)
	huma.Register(api, huma.Operation{
		OperationID: "setJobNotes", Method: http.MethodPatch, Path: "/jobs/{id}/notes",
		Summary: "Set a job's notes", Tags: []string{"jobs"},
	}, rr.handler.setNotes)
	huma.Register(api, huma.Operation{
		OperationID: "bulkDismissJobs", Method: http.MethodPost, Path: "/jobs/dismiss/bulk",
		Summary: "Dismiss multiple jobs at once", Tags: []string{"jobs"},
	}, rr.handler.bulkDismiss)
}
