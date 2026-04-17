package dashboard

import (
	"context"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/infra/kafka"
)

// PipelineStats is the full response returned by GET /dashboard/pipeline.
type PipelineStats struct {
	TotalCompanies     int64 `json:"total_companies"`
	ActiveCompanies    int64 `json:"active_companies"`
	Queued             int64 `json:"queued"`
	Crawling           int64 `json:"crawling"`
	AwaitingFilter     int64 `json:"awaiting_filter"`
	AwaitingEnrichment int64 `json:"awaiting_enrichment"`
	NewJobs24h         int64 `json:"new_jobs_24h"`
	Errors24h          int64 `json:"errors_24h"`
}

// PipelineService assembles DB stats and Kafka lag into PipelineStats.
type PipelineService interface {
	GetPipelineStats(ctx context.Context) (*PipelineStats, error)
}

type pipelineService struct {
	store PipelineStore
	lag   kafka.LagReader
}

// NewPipelineService constructs a PipelineService.
func NewPipelineService(store PipelineStore, lag kafka.LagReader) PipelineService {
	return &pipelineService{store: store, lag: lag}
}

func (s *pipelineService) GetPipelineStats(ctx context.Context) (*PipelineStats, error) {
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

	return &PipelineStats{
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
