package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// PipelineService assembles DB stats and Kafka lag into PipelineStats.
type PipelineService interface {
	GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error)
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
