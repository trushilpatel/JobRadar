package services

import "github.com/go-chi/chi/v5"

// AtsRestRouter mounts the /ats routes. The middleware chain that gates
// these routes is applied by the caller passing a chi.Router subtree that
// already has those middleware in place — the router itself only knows
// route paths and verbs.
type AtsRestRouter struct {
	handler *AtsRestService
}

// NewRestRouter constructs an AtsRestRouter.
func NewRestRouter(h *AtsRestService) *AtsRestRouter {
	return &AtsRestRouter{handler: h}
}

// Register satisfies handler.RouteGroup. Mounts /ats routes; the caller is
// responsible for placing this group under the correct subrouter.
func (rt *AtsRestRouter) Register(r chi.Router) {
	r.Get("/ats", rt.handler.ListATS)
	r.Get("/ats/{name}", rt.handler.GetATS)
	r.Patch("/ats/{name}", rt.handler.UpdateATS)
}
