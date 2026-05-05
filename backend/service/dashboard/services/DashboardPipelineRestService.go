package services

import (
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/handler"
)

// PipelineRestService is the HTTP adapter for the admin pipeline dashboard.
// It exposes a single handler for GET /admin/dashboard/pipeline; routing is
// the responsibility of PipelineRestRouter.
type PipelineRestService struct {
	next PipelineService
}

// NewPipelineRestService constructs the pipeline HTTP adapter.
func NewPipelineRestService(next PipelineService) *PipelineRestService {
	if next == nil {
		panic("dashboard.NewPipelineRestService: next must not be nil")
	}
	return &PipelineRestService{next: next}
}

// getPipelineStats handles GET /admin/dashboard/pipeline.
func (h *PipelineRestService) getPipelineStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.next.GetPipelineStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}
