// Package services holds the decorator chain for the pipeline domain.
//
// The chain composes RestRouter → RestService → ValidationService →
// ImplService → RepositoryService. Each layer overrides only the methods it
// owns; everything else auto-delegates via BasePipelineService so adding a
// method to the interface costs one line per layer that cares.
package services

import (
	"context"

	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
)

// PipelineService is the contract every pipeline-domain layer honours.
//
// GetStats returns a live snapshot of pipeline health across the scan
// queue, filter/enrichment backlog, and recent throughput/error counts.
// TriggerRetry validates a (worker, tier) redelivery request; NATS
// JetStream's BackOff/MaxDeliver handles actual redelivery automatically.
type PipelineService interface {
	GetStats(ctx context.Context) (pipelinedto.PipelineStats, error)
	TriggerRetry(ctx context.Context, input pipelinedto.TriggerRetryInput) (pipelinedto.TriggerRetryResult, error)
}

// BasePipelineService auto-delegates every PipelineService method to next.
// Layers embed it and override only the methods they have logic for; the
// rest fall through.
type BasePipelineService struct {
	next PipelineService
}

// NewBasePipelineService wraps next. Panics if next is nil — a misconfigured
// chain is a programmer error, never a runtime condition we want to handle.
func NewBasePipelineService(next PipelineService) BasePipelineService {
	if next == nil {
		panic("pipeline.BasePipelineService: next must not be nil — check wire setup")
	}
	return BasePipelineService{next: next}
}

func (b *BasePipelineService) GetStats(ctx context.Context) (pipelinedto.PipelineStats, error) {
	return b.next.GetStats(ctx)
}

func (b *BasePipelineService) TriggerRetry(
	ctx context.Context,
	input pipelinedto.TriggerRetryInput,
) (pipelinedto.TriggerRetryResult, error) {
	return b.next.TriggerRetry(ctx, input)
}
