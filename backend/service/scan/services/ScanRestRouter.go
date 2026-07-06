package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// RestRouter registers scan routes via Huma.
// It owns all routing logic; RestService owns only Huma operation handlers.
type RestRouter struct {
	handler *RestService
	zone    huma.Config
}

// NewRestRouter constructs a RestRouter. zone is the shared /api
// middleware-zone Huma config (see apihuma.NewZoneConfig / apispec.APIZoneConfig).
func NewRestRouter(h *RestService, zone huma.Config) *RestRouter {
	return &RestRouter{handler: h, zone: zone}
}

// Register mounts scan-specific routes under /companies.
func (rt *RestRouter) Register(router chi.Router) {
	api := humachi.New(router, rt.zone)

	huma.Register(api, huma.Operation{
		OperationID: "listScanQueue", Method: http.MethodGet, Path: "/companies/queue",
		Summary: "List the company scan queue", Tags: []string{"scan"},
	}, rt.handler.ListQueue)
	huma.Register(api, huma.Operation{
		OperationID: "removeFromScanQueue", Method: http.MethodDelete, Path: "/companies/queue/{entry_id}",
		Summary: "Remove an entry from the scan queue", Tags: []string{"scan"},
	}, rt.handler.RemoveFromQueue)
	huma.Register(api, huma.Operation{
		OperationID: "triggerScanAll", Method: http.MethodPost, Path: "/companies/scan-all",
		Summary: "Trigger a scan for every active company", Tags: []string{"scan"},
		DefaultStatus: http.StatusAccepted,
	}, rt.handler.TriggerScanAll)
	huma.Register(api, huma.Operation{
		OperationID: "getScanStats", Method: http.MethodGet, Path: "/companies/{id}/scan-stats",
		Summary: "Get scan stats for a company", Tags: []string{"scan"},
	}, rt.handler.ScanStats)
	huma.Register(api, huma.Operation{
		OperationID: "triggerScan", Method: http.MethodPost, Path: "/companies/{id}/scan",
		Summary: "Trigger a scan for a company", Tags: []string{"scan"},
		DefaultStatus: http.StatusAccepted,
	}, rt.handler.TriggerScan)
	huma.Register(api, huma.Operation{
		OperationID: "getCompanyAnalytics", Method: http.MethodGet, Path: "/companies/{id}/analytics",
		Summary: "Get analytics for a company", Tags: []string{"scan"},
	}, rt.handler.Analytics)
	huma.Register(api, huma.Operation{
		OperationID: "getEffectiveScanConfig", Method: http.MethodGet, Path: "/companies/{id}/scan-config",
		Summary: "Get the effective scan config for a company", Tags: []string{"scan"},
	}, rt.handler.GetEffectiveScanConfig)
	huma.Register(api, huma.Operation{
		OperationID: "updateScanIntervalOverride", Method: http.MethodPatch, Path: "/companies/{id}/scan-interval-override",
		Summary: "Set or clear a company's scan interval override", Tags: []string{"scan"},
	}, rt.handler.UpdateScanIntervalOverride)
}
