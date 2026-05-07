package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// retryTarget maps (worker, tier) to the Kafka group + topics for draining.
type retryTarget struct {
	groupID string
	src     string
	dst     string
}

// retryTargets is keyed by worker → tier → target.
var retryTargets = map[string]map[string]retryTarget{
	"filter": {
		"1m":  {"job-listing-relevance-filter-retry-1m", kafka.TopicJobListingsRawRetry1m, kafka.TopicJobListingsRaw},
		"10m": {"job-listing-relevance-filter-retry-10m", kafka.TopicJobListingsRawRetry10m, kafka.TopicJobListingsRaw},
		"1h":  {"job-listing-relevance-filter-retry-1h", kafka.TopicJobListingsRawRetry1h, kafka.TopicJobListingsRaw},
		"dlq": {"jobradar-dlq-observer", kafka.TopicJobListingsRawDLQ, kafka.TopicJobListingsRaw},
	},
	"enricher": {
		"1m":  {"job-post-analyzer-retry-1m", kafka.TopicJobListingsFilteredRetry1m, kafka.TopicJobListingsFiltered},
		"10m": {"job-post-analyzer-retry-10m", kafka.TopicJobListingsFilteredRetry10m, kafka.TopicJobListingsFiltered},
		"1h":  {"job-post-analyzer-retry-1h", kafka.TopicJobListingsFilteredRetry1h, kafka.TopicJobListingsFiltered},
		"dlq": {"jobradar-dlq-observer", kafka.TopicJobListingsFilteredDLQ, kafka.TopicJobListingsFiltered},
	},
}

// PipelineImplService implements PipelineService.
type PipelineImplService struct {
	store   PipelineStore
	lag     kafka.LagReader
	drainer kafka.RetryDrainer
}

// NewPipelineService constructs a PipelineService.
func NewPipelineService(store PipelineStore, lag kafka.LagReader, drainer kafka.RetryDrainer) PipelineService {
	return &PipelineImplService{store: store, lag: lag, drainer: drainer}
}

func (s *PipelineImplService) GetPipelineStats(ctx context.Context) (*dto.PipelineStats, error) {
	dbStats, err := s.store.GetPipelineDBStats(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch pipeline stats")
	}

	// Use DB-based count as primary source for awaiting_filter: jobs that have been
	// crawled (reason='fetch') but are not yet in `jobs`. This is more reliable
	// than Kafka consumer-group lag which returns 0 when workers haven't committed offsets.
	awaitingFilter := dbStats.PipelinePending

	awaitingEnrichment, err := s.lag.GetLag(ctx, "job-post-analyzer", kafka.TopicJobListingsFiltered)
	if err != nil {
		awaitingEnrichment = 0
	}

	getLag := func(groupID, topic string) int64 {
		n, _ := s.lag.GetLag(ctx, groupID, topic)
		return n
	}

	return &dto.PipelineStats{
		TotalCompanies:  dbStats.TotalCompanies,
		ActiveCompanies: dbStats.ActiveCompanies,
		Queued:          dbStats.Queued,
		Crawling:        dbStats.Crawling,
		AwaitingFilter:  awaitingFilter,
		FilterRetries: dto.RetryTierCounts{
			Tier1m:  getLag("job-listing-relevance-filter-retry-1m", kafka.TopicJobListingsRawRetry1m),
			Tier10m: getLag("job-listing-relevance-filter-retry-10m", kafka.TopicJobListingsRawRetry10m),
			Tier1h:  getLag("job-listing-relevance-filter-retry-1h", kafka.TopicJobListingsRawRetry1h),
			DLQ:     getLag("jobradar-dlq-observer", kafka.TopicJobListingsRawDLQ),
		},
		AwaitingEnrichment: awaitingEnrichment,
		EnricherRetries: dto.RetryTierCounts{
			Tier1m:  getLag("job-post-analyzer-retry-1m", kafka.TopicJobListingsFilteredRetry1m),
			Tier10m: getLag("job-post-analyzer-retry-10m", kafka.TopicJobListingsFilteredRetry10m),
			Tier1h:  getLag("job-post-analyzer-retry-1h", kafka.TopicJobListingsFilteredRetry1h),
			DLQ:     getLag("jobradar-dlq-observer", kafka.TopicJobListingsFilteredDLQ),
		},
		NewJobs24h: dbStats.NewJobs24h,
		Errors24h:  dbStats.Errors24h,
	}, nil
}

func (s *PipelineImplService) TriggerRetry(ctx context.Context, worker, tier string) (int64, error) {
	workers, ok := retryTargets[worker]
	if !ok {
		return 0, errors.ErrBadRequest.WithMessage("unknown worker: " + worker)
	}
	target, ok := workers[tier]
	if !ok {
		return 0, errors.ErrBadRequest.WithMessage("unknown tier: " + tier)
	}
	n, err := s.drainer.Drain(ctx, target.groupID, target.src, target.dst)
	if err != nil {
		return 0, errors.ErrInternal.WithMessage("drain failed: " + err.Error())
	}
	return n, nil
}
