package services

import (
	"context"

	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/response"
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

// triggerRetryInput is the Huma input for POST /pipeline/retry.
type triggerRetryInput struct {
	Body pipelinedto.TriggerRetryInput
}

// getStats handles GET /pipeline/stats — returns a live pipeline health snapshot.
func (h *PipelineRestService) getStats(ctx context.Context, _ *struct{}) (*apihuma.SingleOutput[pipelinedto.PipelineStats], error) {
	stats, err := h.next.GetStats(ctx)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[pipelinedto.PipelineStats]{}
	out.Body = response.Response[pipelinedto.PipelineStats]{Data: stats}
	return out, nil
}

// triggerRetry handles POST /pipeline/retry — validates a (worker, tier)
// redelivery request.
func (h *PipelineRestService) triggerRetry(ctx context.Context, in *triggerRetryInput) (*apihuma.SingleOutput[pipelinedto.TriggerRetryResult], error) {
	result, err := h.next.TriggerRetry(ctx, in.Body)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[pipelinedto.TriggerRetryResult]{}
	out.Body = response.Response[pipelinedto.TriggerRetryResult]{Data: result}
	return out, nil
}
