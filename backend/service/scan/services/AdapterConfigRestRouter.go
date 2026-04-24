package services

import "github.com/go-chi/chi/v5"

// AdapterConfigRestRouter registers adapter-config routes onto a chi.Router.
// It owns all routing logic; AdapterConfigRestService owns only HTTP handler methods.
type AdapterConfigRestRouter struct {
	handler *AdapterConfigRestService
}

// NewAdapterConfigRestRouter constructs an AdapterConfigRestRouter.
func NewAdapterConfigRestRouter(h *AdapterConfigRestService) *AdapterConfigRestRouter {
	return &AdapterConfigRestRouter{handler: h}
}

// Register mounts adapter-config and company scan-config routes onto the router.
func (r *AdapterConfigRestRouter) Register(router chi.Router) {
	router.Get("/adapter-configs", r.handler.ListAdapterConfigs)
	router.Patch("/adapter-configs/{adapter_type}", r.handler.UpdateAdapterConfig)
	router.Patch("/companies/{id}/scan-interval-override", r.handler.UpdateScanIntervalOverride)
	router.Get("/companies/{id}/scan-config", r.handler.GetEffectiveScanConfig)
}
