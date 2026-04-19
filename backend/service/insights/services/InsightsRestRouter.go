package services

import "github.com/go-chi/chi/v5"

// RestRouter registers all insights routes onto a chi.Router.
type RestRouter struct {
	handler *RestService
}

// NewRestRouter constructs a RestRouter.
func NewRestRouter(h *RestService) *RestRouter {
	return &RestRouter{handler: h}
}

// Register mounts all insights routes onto r.
func (r *RestRouter) Register(router chi.Router) {
	router.Get("/insights/tech-stack", r.handler.TechStack)
	router.Get("/insights/company-activity", r.handler.CompanyActivity)
	router.Get("/insights/funnel", r.handler.Funnel)
	router.Get("/insights/location", r.handler.Location)
	router.Get("/insights/sponsorship", r.handler.Sponsorship)
	router.Get("/insights/velocity", r.handler.Velocity)
	router.Get("/insights/role-categories", r.handler.RoleCategories)
}
