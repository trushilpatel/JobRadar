package services

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	insightsdto "github.com/trushilpatel/jobradar/service/insights/dto"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// DashboardImplService implements the DashboardService interface.
type DashboardImplService struct {
	scanLog     scansvc.ScanLogReader
	insightsSvc insightssvc.InsightsService
	jobCounts   JobCountReader
}

// NewDashboardImplService constructs a DashboardImplService implementation.
func NewDashboardImplService(scanLog scansvc.ScanLogReader, insightsSvc insightssvc.InsightsService, jobCounts JobCountReader) DashboardService {
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

	var (
		scannedToday                       int64
		newCount, savedCount, appliedCount int64
		pipeline                           insightsdto.PipelineCounts
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		v, e := s.scanLog.GetJobsScannedToday(gctx)
		if e != nil {
			return errors.ErrInternal.WithMessage("failed to fetch scanned today")
		}
		scannedToday = v
		return nil
	})

	g.Go(func() error {
		n, sv, ap, e := s.jobCounts.GetJobCounts()
		if e != nil {
			return errors.ErrInternal.WithMessage("failed to fetch job counts")
		}
		newCount, savedCount, appliedCount = n, sv, ap
		return nil
	})

	g.Go(func() error {
		p, e := s.insightsSvc.GetPipelineCounts(gctx)
		if e != nil {
			return errors.ErrInternal.WithMessage("failed to fetch pipeline counts")
		}
		pipeline = p
		return nil
	})

	if err = g.Wait(); err != nil {
		return nil, err
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
		return nil, errors.ErrInternal.WithMessage("failed to fetch activity")
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
