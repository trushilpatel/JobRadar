package services

import "github.com/go-chi/chi/v5"

// RestRouter registers company routes onto a chi.Router.
// It owns all routing logic; RestService owns only HTTP handler methods.
type RestRouter struct {
	handler *RestService
}

// NewRestRouter constructs a RestRouter.
func NewRestRouter(h *RestService) *RestRouter {
	return &RestRouter{handler: h}
}

// Register mounts all company routes onto router.
func (rr *RestRouter) Register(router chi.Router) {
	router.Route("/companies", func(r chi.Router) {
		r.Get("/", rr.handler.List)
		r.Post("/", rr.handler.Create)
		r.Patch("/active", rr.handler.SetAllActive)
		r.Patch("/{id}", rr.handler.Update)
		r.Delete("/{id}", rr.handler.Delete)
	})
}
