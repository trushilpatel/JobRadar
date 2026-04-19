package services

import "github.com/go-chi/chi/v5"

// RestRouter registers scan routes onto a chi.Router.
// It owns all routing logic; RestService owns only HTTP handler methods.
type RestRouter struct {
	handler *RestService
}

// NewRestRouter constructs a RestRouter.
func NewRestRouter(h *RestService) *RestRouter {
	return &RestRouter{handler: h}
}

// Register mounts scan-specific routes under /companies.
func (r *RestRouter) Register(router chi.Router) {
	router.Get("/companies/queue", r.handler.ListQueue)
	router.Delete("/companies/queue/{entry_id}", r.handler.RemoveFromQueue)
	router.Post("/companies/queue/{entry_id}/move-up", r.handler.MoveQueueItemUp)
	router.Post("/companies/queue/{entry_id}/move-down", r.handler.MoveQueueItemDown)
	router.Post("/companies/scan-all", r.handler.TriggerScanAll)
	router.Get("/companies/{id}/scan-stats", r.handler.ScanStats)
	router.Post("/companies/{id}/scan", r.handler.TriggerScan)
}
