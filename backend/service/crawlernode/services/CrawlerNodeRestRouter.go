package services

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NodeRestRouter wires the node-facing endpoints onto a chi router.
// Route registration only — no business logic, no parsing, no responses.
type NodeRestRouter struct {
	svc    *NodeRestService
	authMw func(http.Handler) http.Handler
}

// NewNodeRestRouter constructs the node-facing router.
// authMw is the RequireNodeToken middleware that gates all /node/* routes.
func NewNodeRestRouter(svc *NodeRestService, authMw func(http.Handler) http.Handler) *NodeRestRouter {
	return &NodeRestRouter{svc: svc, authMw: authMw}
}

// Register mounts node-facing routes onto r. Expected to be called under /node.
func (rt *NodeRestRouter) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(rt.authMw)
		r.Get("/work", rt.svc.GetWork)
		r.Post("/results", rt.svc.SubmitResults)
		r.Post("/error", rt.svc.ReportError)
	})
}

// AdminRestRouter wires the admin-facing crawler-node endpoints onto a chi router.
type AdminRestRouter struct {
	svc *AdminRestService
}

// NewAdminRestRouter constructs the admin-facing router.
func NewAdminRestRouter(svc *AdminRestService) *AdminRestRouter {
	return &AdminRestRouter{svc: svc}
}

// Register mounts admin-facing routes onto r. Expected to be called under /admin/crawler-nodes.
func (rt *AdminRestRouter) Register(r chi.Router) {
	r.Get("/", rt.svc.ListNodes)
	r.Post("/", rt.svc.ProvisionNode)
	r.Patch("/{id}", rt.svc.SetNodeStatus)
	r.Delete("/{id}", rt.svc.RetireNode)
}
