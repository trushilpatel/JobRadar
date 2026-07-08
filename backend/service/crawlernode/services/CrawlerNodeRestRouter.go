package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// NodeRestRouter wires the node-facing endpoints onto a chi router via Huma.
// Route registration only — no business logic, no parsing, no responses.
type NodeRestRouter struct {
	svc    *NodeRestService
	authMw func(http.Handler) http.Handler
	zone   huma.Config
}

// NewNodeRestRouter constructs the node-facing router. authMw is the
// RequireNodeToken middleware that gates all /node/* routes. zone is the
// orchestrator's node-zone Huma config (see apps/orchestrator's spec
// wiring) — humachi.New still runs every chi middleware already attached
// via .Use() on the router it's bound to (confirmed in the original Phase 0
// spike, docs/superpowers/plans/2026-06-13-huma-migration-plan.md Section 3.0),
// so applying authMw before humachi.New works exactly like the pre-Huma group.
func NewNodeRestRouter(svc *NodeRestService, authMw func(http.Handler) http.Handler, zone huma.Config) *NodeRestRouter {
	return &NodeRestRouter{svc: svc, authMw: authMw, zone: zone}
}

// Register mounts node-facing routes onto r. Expected to be called under /node.
func (rt *NodeRestRouter) Register(r chi.Router) {
	r.Group(func(sub chi.Router) {
		sub.Use(rt.authMw)
		api := humachi.New(sub, rt.zone)
		huma.Register(api, huma.Operation{
			OperationID: "getWork", Method: http.MethodGet, Path: "/work",
			Summary: "Claim work for the calling crawler node", Tags: []string{"node"},
		}, rt.svc.getWork)
		huma.Register(api, huma.Operation{
			OperationID: "submitResults", Method: http.MethodPost, Path: "/results",
			Summary: "Submit crawl results for an assignment", Tags: []string{"node"},
		}, rt.svc.submitResults)
		huma.Register(api, huma.Operation{
			OperationID: "reportError", Method: http.MethodPost, Path: "/error",
			Summary: "Report a crawl error for an assignment", Tags: []string{"node"},
		}, rt.svc.reportError)
	})
}

// AdminRestRouter wires the admin-facing crawler-node endpoints onto a chi
// router via Huma.
type AdminRestRouter struct {
	svc  *AdminRestService
	zone huma.Config
}

// NewAdminRestRouter constructs the admin-facing router. zone is the
// orchestrator's admin-zone Huma config.
func NewAdminRestRouter(svc *AdminRestService, zone huma.Config) *AdminRestRouter {
	return &AdminRestRouter{svc: svc, zone: zone}
}

// Register mounts admin-facing routes onto r. Expected to be called under
// /admin/crawler-nodes (behind the admin auth gate — see
// apps/orchestrator/wire_admin_gate.go).
func (rt *AdminRestRouter) Register(r chi.Router) {
	api := humachi.New(r, rt.zone)
	huma.Register(api, huma.Operation{
		OperationID: "listCrawlerNodes", Method: http.MethodGet, Path: "/",
		Summary: "List crawler nodes", Tags: []string{"crawler-nodes"},
	}, rt.svc.listNodes)
	huma.Register(api, huma.Operation{
		OperationID: "provisionCrawlerNode", Method: http.MethodPost, Path: "/",
		Summary: "Provision a new crawler node", Tags: []string{"crawler-nodes"},
		DefaultStatus: http.StatusCreated,
	}, rt.svc.provisionNode)
	huma.Register(api, huma.Operation{
		OperationID: "setCrawlerNodeStatus", Method: http.MethodPatch, Path: "/{id}",
		Summary: "Set a crawler node's status", Tags: []string{"crawler-nodes"},
	}, rt.svc.setNodeStatus)
	huma.Register(api, huma.Operation{
		OperationID: "retireCrawlerNode", Method: http.MethodDelete, Path: "/{id}",
		Summary: "Retire a crawler node", Tags: []string{"crawler-nodes"},
	}, rt.svc.retireNode)
}
