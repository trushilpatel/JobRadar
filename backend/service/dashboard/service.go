package dashboard

import (
	"context"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/service/insights"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// Service defines all dashboard aggregation operations.
type Service interface {
	GetStats(ctx context.Context) (*StatsView, error)
	GetActivity(ctx context.Context, days int) ([]ActivityView, error)
}

type service struct {
	scanLog     scansvc.ScanLogReader
	insightsSvc insights.Service
	jobCounts   JobCountReader
}

// JobCountReader is the minimal interface needed to get job counts.
// GetJobCounts returns new, saved, applied counts respectively.
type JobCountReader interface {
	GetJobCounts() (newCount, savedCount, appliedCount int64, err error)
}

// NewService constructs a Service implementation.
func NewService(scanLog scansvc.ScanLogReader, insightsSvc insights.Service, jobCounts JobCountReader) Service {
	return &service{
		scanLog:     scanLog,
		insightsSvc: insightsSvc,
		jobCounts:   jobCounts,
	}
}

// GetStats returns aggregated dashboard statistics.
func (s *service) GetStats(ctx context.Context) (_ *StatsView, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.service.GetStats")
	defer tracing.End(span, &err)
	scannedToday, err := s.scanLog.GetJobsScannedToday(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch scanned today")
	}

	newCount, savedCount, appliedCount, err := s.jobCounts.GetJobCounts()
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch job counts")
	}

	pipeline, err := s.insightsSvc.GetPipelineCounts(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch pipeline counts")
	}

	relevant := newCount + savedCount + appliedCount

	return &StatsView{
		JobsScannedToday: scannedToday,
		JobsNew:          newCount,
		JobsSaved:        savedCount,
		JobsApplied:      appliedCount,
		PipelinePending:  pipeline.Pending,
		PipelineErrors:   pipeline.Errors,
		Funnel: FunnelSummary{
			Scanned:  scannedToday,
			Relevant: relevant,
			Saved:    savedCount,
			Applied:  appliedCount,
		},
	}, nil
}

// GetActivity returns per-day scan activity for the given number of days.
func (s *service) GetActivity(ctx context.Context, days int) (_ []ActivityView, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.service.GetActivity")
	defer tracing.End(span, &err)
	scans, err := s.scanLog.GetScansByDay(ctx, days)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch activity")
	}

	views := make([]ActivityView, len(scans))
	for i, sc := range scans {
		views[i] = ActivityView{
			Date:    sc.Date,
			Scanned: sc.Scanned,
			NewJobs: sc.NewJobs,
		}
	}
	return views, nil
}
