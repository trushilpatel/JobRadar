package services

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RestRouter mounts the /api/me routes. The middleware chain that gates
// these routes (auth, RequireIdentity, rate limit) is applied by the caller
// passing a chi.Router subtree that already has those middleware in place —
// the router itself only knows route paths and verbs.
type RestRouter struct {
	handler     *RestService
	middlewares []func(http.Handler) http.Handler
}

// NewRestRouter constructs a RestRouter. The variadic middlewares are
// applied to the /api/me subtree only — handlers on the public router stay
// unaffected. Pass them in this order: RequireIdentity, RateLimit.
func NewRestRouter(h *RestService, middlewares ...func(http.Handler) http.Handler) *RestRouter {
	return &RestRouter{handler: h, middlewares: middlewares}
}

// Register satisfies handler.RouteGroup. Mounts /me routes; the caller is
// responsible for placing this group under the /api subrouter.
func (rt *RestRouter) Register(r chi.Router) {
	r.Route("/me", func(sub chi.Router) {
		for _, mw := range rt.middlewares {
			if mw != nil {
				sub.Use(mw)
			}
		}
		sub.Get("/", rt.handler.getMe)
		sub.Patch("/", rt.handler.updateMe)
		sub.Delete("/", rt.handler.deleteMe)
	})
}
