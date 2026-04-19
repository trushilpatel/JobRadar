package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// PipelineService assembles DB stats and Kafka lag into PipelineStats.
type PipelineService interface {
	GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error)
}
