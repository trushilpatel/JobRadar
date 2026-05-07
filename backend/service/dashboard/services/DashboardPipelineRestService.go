package services

import (
	"encoding/json"
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// PipelineRestService is the HTTP adapter for the admin pipeline dashboard.
// It exposes handlers for GET /admin/dashboard/pipeline and
// POST /admin/dashboard/pipeline/retry; routing is the responsibility of
// PipelineRestRouter.
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

// triggerRetry handles POST /admin/dashboard/pipeline/retry.
func (h *PipelineRestService) triggerRetry(w http.ResponseWriter, r *http.Request) {
	var req dto.TriggerRetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handler.HandleError(w, errors.ErrBadRequest.WithMessage("invalid request body"))
		return
	}
	n, err := h.next.TriggerRetry(r.Context(), req.Worker, req.Tier)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, dto.TriggerRetryResponse{Drained: n})
}
