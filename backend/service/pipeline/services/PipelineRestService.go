package services

import (
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
)

// PipelineRestService is the HTTP adapter for the pipeline domain. It owns
// request decoding and response shaping for the two /pipeline endpoints;
// routing is the responsibility of PipelineRestRouter.
//
// PipelineRestService does NOT implement PipelineService — it consumes one.
// Per the slice convention, next is its only dependency.
type PipelineRestService struct {
	next PipelineService
}

// NewRestService constructs the HTTP adapter.
func NewRestService(next PipelineService) *PipelineRestService {
	if next == nil {
		panic("pipeline.NewRestService: next must not be nil")
	}
	return &PipelineRestService{next: next}
}

// GetStats handles GET /pipeline/stats — returns a live pipeline health snapshot.
func (h *PipelineRestService) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.next.GetStats(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, stats)
}

// TriggerRetry handles POST /pipeline/retry — validates a (worker, tier)
// redelivery request.
func (h *PipelineRestService) TriggerRetry(w http.ResponseWriter, r *http.Request) {
	input, ok := handler.DecodeBody[pipelinedto.TriggerRetryInput](w, r)
	if !ok {
		return
	}

	result, err := h.next.TriggerRetry(r.Context(), input)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, result)
}
