package services

import (
	"context"
	"fmt"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	insightsdto "github.com/trushilpatel/jobradar/service/insights/dto"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

var _ DashboardService = (*DashboardImplService)(nil)

// pipelineCountReader is the narrow interface this service needs from insights.
type pipelineCountReader interface {
	GetPipelineCounts(ctx context.Context) (insightsdto.PipelineCounts, error)
}

// DashboardImplService implements the DashboardService interface.
type DashboardImplService struct {
	scanLog  scansvc.ScanLogReader
	pipeline pipelineCountReader
}

// NewDashboardImplService constructs a DashboardImplService implementation.
func NewDashboardImplService(scanLog scansvc.ScanLogReader, pipeline pipelineCountReader) DashboardService {
	return &DashboardImplService{
		scanLog:  scanLog,
		pipeline: pipeline,
	}
}

// GetStats returns aggregated dashboard statistics.
func (s *DashboardImplService) GetStats(ctx context.Context) (_ *dto.StatsView, err error) {
	ctx, span := tracing.Start(ctx, "dashboard.service.GetStats")
	defer tracing.End(span, &err)

	scannedToday, e := s.scanLog.GetJobsScannedToday(ctx)
	if e != nil {
		return nil, errors.ErrInternal.WithMessage(fmt.Sprintf("failed to fetch scanned today: %v", e))
	}

	pipeline, e := s.pipeline.GetPipelineCounts(ctx)
	if e != nil {
		return nil, errors.ErrInternal.WithMessage(fmt.Sprintf("failed to fetch pipeline counts: %v", e))
	}

	return &dto.StatsView{
		JobsScannedToday: scannedToday,
		PipelinePending:  pipeline.Pending,
		PipelineErrors:   pipeline.Errors,
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
		views[i] = dto.ActivityView{Date: sc.Date, Scanned: sc.Scanned, NewJobs: sc.NewJobs}
	}
	return views, nil
}
