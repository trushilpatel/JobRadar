package services

import "github.com/go-chi/chi/v5"

// RestRouter registers all dashboard routes onto a chi.Router.
type RestRouter struct {
	handler *RestService
}

// NewRestRouter constructs a RestRouter.
func NewRestRouter(h *RestService) *RestRouter {
	return &RestRouter{handler: h}
}

// Register mounts all dashboard routes onto r.
func (r *RestRouter) Register(router chi.Router) {
	router.Get("/dashboard/stats", r.handler.Stats)
	router.Get("/dashboard/activity", r.handler.Activity)
	router.Get("/dashboard/progress", r.handler.Progress)
	router.Get("/dashboard/market", r.handler.Market)
	router.Get("/dashboard/heatmap", r.handler.Heatmap)
	router.Get("/dashboard/interviews", r.handler.Interviews)
	router.Get("/dashboard/velocity", r.handler.Velocity)
	router.Get("/dashboard/tech-trends", r.handler.TechTrends)
	router.Get("/dashboard/pipeline", r.handler.Pipeline)
}
