package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// PipelineImplService implements PipelineService.
type PipelineImplService struct {
	store PipelineStore
	lag   kafka.LagReader
}

// NewPipelineService constructs a PipelineService.
func NewPipelineService(store PipelineStore, lag kafka.LagReader) PipelineService {
	return &PipelineImplService{store: store, lag: lag}
}

func (s *PipelineImplService) GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error) {
	dbStats, err := s.store.GetPipelineDBStats(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch pipeline stats")
	}

	// Use DB-based count as primary source for awaiting_filter: jobs that have been
	// crawled (reason='fetch') but are not yet in job_feed. This is more reliable
	// than Kafka consumer-group lag which returns 0 when workers haven't committed offsets.
	awaitingFilter := dbStats.PipelinePending

	awaitingEnrichment, err := s.lag.GetLag(ctx, "job-post-analyzer", kafka.TopicJobListingsFiltered)
	if err != nil {
		awaitingEnrichment = 0
	}

	return &dto.PipelineStats{
		TotalCompanies:     dbStats.TotalCompanies,
		ActiveCompanies:    dbStats.ActiveCompanies,
		Queued:             dbStats.Queued,
		Crawling:           dbStats.Crawling,
		AwaitingFilter:     awaitingFilter,
		AwaitingEnrichment: awaitingEnrichment,
		NewJobs24h:         dbStats.NewJobs24h,
		Errors24h:          dbStats.Errors24h,
	}, nil
}
