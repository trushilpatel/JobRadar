package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
)

// validRetryTargets maps worker → tier for input validation.
// NATS BackOff/MaxDeliver handles redelivery automatically — no manual drain.
var validRetryTargets = map[string]map[string]struct{}{
	"filter": {
		"1m": {}, "10m": {}, "1h": {}, "dlq": {},
	},
	"enricher": {
		"1m": {}, "10m": {}, "1h": {}, "dlq": {},
	},
}

// PipelineImplService implements PipelineService.
type PipelineImplService struct {
	store PipelineStore
}

// NewPipelineService constructs a PipelineService.
func NewPipelineService(store PipelineStore) PipelineService {
	return &PipelineImplService{store: store}
}

func (s *PipelineImplService) GetPipelineStats(ctx context.Context) (_ *dto.PipelineStats, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.pipeline.GetPipelineStats")
	defer tracing.End(span, &err)
	dbStats, err := s.store.GetPipelineDBStats(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch pipeline stats")
	}
	return &dto.PipelineStats{
		TotalCompanies:     dbStats.TotalCompanies,
		ActiveCompanies:    dbStats.ActiveCompanies,
		Queued:             dbStats.Queued,
		Crawling:           dbStats.Crawling,
		AwaitingFilter:     dbStats.PipelinePending,
		FilterRetries:      dto.RetryTierCounts{},
		AwaitingEnrichment: dbStats.PipelineFiltered,
		EnricherRetries:    dto.RetryTierCounts{},
		NewJobs24h:         dbStats.NewJobs24h,
		Errors24h:          dbStats.Errors24h,
	}, nil
}

func (s *PipelineImplService) TriggerRetry(ctx context.Context, worker, tier string) (_ int64, err error) {
	_, span := tracing.Start(ctx, "dashboard.pipeline.TriggerRetry")
	defer tracing.End(span, &err)
	tiers, ok := validRetryTargets[worker]
	if !ok {
		return 0, errors.ErrBadRequest.WithMessage("unknown worker")
	}
	if _, ok := tiers[tier]; !ok {
		return 0, errors.ErrBadRequest.WithMessage("unknown tier")
	}
	// NATS redelivers via BackOff/MaxDeliver — no manual drain needed.
	return 0, nil
}
