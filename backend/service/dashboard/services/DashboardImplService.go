package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// DashboardImplService implements the DashboardService interface.
type DashboardImplService struct {
	scanLog     scansvc.ScanLogReader
	insightsSvc insightssvc.Service
	jobCounts   JobCountReader
}

// NewDashboardImplService constructs a DashboardImplService implementation.
func NewDashboardImplService(scanLog scansvc.ScanLogReader, insightsSvc insightssvc.Service, jobCounts JobCountReader) DashboardService {
	return &DashboardImplService{
		scanLog:     scanLog,
		insightsSvc: insightsSvc,
		jobCounts:   jobCounts,
	}
}

// GetStats returns aggregated dashboard statistics.
func (s *DashboardImplService) GetStats(ctx context.Context) (_ *dto.StatsView, err error) {
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

	return &dto.StatsView{
		JobsScannedToday: scannedToday,
		JobsNew:          newCount,
		JobsSaved:        savedCount,
		JobsApplied:      appliedCount,
		PipelinePending:  pipeline.Pending,
		PipelineErrors:   pipeline.Errors,
		Funnel: dto.FunnelSummary{
			Scanned:  scannedToday,
			Relevant: relevant,
			Saved:    savedCount,
			Applied:  appliedCount,
		},
	}, nil
}

// GetActivity returns per-day scan activity for the given number of days.
func (s *DashboardImplService) GetActivity(ctx context.Context, days int) (_ []dto.ActivityView, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.service.GetActivity")
	defer tracing.End(span, &err)
	scans, err := s.scanLog.GetScansByDay(ctx, days)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch activity")
	}

	views := make([]dto.ActivityView, len(scans))
	for i, sc := range scans {
		views[i] = dto.ActivityView{
			Date:    sc.Date,
			Scanned: sc.Scanned,
			NewJobs: sc.NewJobs,
		}
	}
	return views, nil
}
