package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// RestRouter registers the apply-queue routes via Huma. Mounted under /api
// by apps/api/router.go, which already applies RequireIdentity + RateLimit
// to every /api/* route — no admin gating needed for this user-scoped
// feature.
type RestRouter struct {
	handler *RestService
	zone    huma.Config
}

// NewRestRouter constructs a RestRouter. zone is the shared /api
// middleware-zone Huma config (see apihuma.NewZoneConfig / apispec.APIZoneConfig).
func NewRestRouter(h *RestService, zone huma.Config) *RestRouter {
	return &RestRouter{handler: h, zone: zone}
}

// Register mounts the apply-queue routes onto r.
func (rr *RestRouter) Register(r chi.Router) {
	api := humachi.New(r, rr.zone)

	huma.Register(api, huma.Operation{
		OperationID: "listApplyQueue", Method: http.MethodGet, Path: "/apply-queue",
		Summary: "List the current user's apply queue", Tags: []string{"apply-queue"},
	}, rr.handler.list)
	huma.Register(api, huma.Operation{
		OperationID: "setApplyQueueResume", Method: http.MethodPut, Path: "/apply-queue/{jobId}/resume",
		Summary: "Attach or clear a resume for a job in the apply queue", Tags: []string{"apply-queue"},
	}, rr.handler.setResume)
}
