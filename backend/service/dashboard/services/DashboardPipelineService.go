package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// PipelineService assembles DB stats and Kafka lag into PipelineStats and
// handles force-retry drain requests.
type PipelineService interface {
	GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error)
	TriggerRetry(ctx context.Context, worker, tier string) (int64, error)
}

// PipelineBaseService delegates all PipelineService methods to next.
type PipelineBaseService struct{ next PipelineService }

// NewPipelineBaseService constructs a PipelineBaseService.
func NewPipelineBaseService(next PipelineService) PipelineBaseService {
	if next == nil {
		panic("PipelineBaseService: next must not be nil")
	}
	return PipelineBaseService{next: next}
}

func (b *PipelineBaseService) GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error) {
	return b.next.GetPipelineStats(ctx)
}

func (b *PipelineBaseService) TriggerRetry(ctx context.Context, worker, tier string) (int64, error) {
	return b.next.TriggerRetry(ctx, worker, tier)
}
