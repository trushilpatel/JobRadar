package dashboard_test

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
	insightsdto "github.com/trushilpatel/jobradar/service/insights/dto"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeScanLogReader is a test double for scan.ScanLogReader.
type fakeScanLogReader struct {
	scannedToday int64
	scansByDay   []dto.DailyScanCount
	err          error
}

func (f *fakeScanLogReader) GetScanStats(_ context.Context, _ string) (dto.ScanStatsView, error) {
	return dto.ScanStatsView{}, f.err
}
func (f *fakeScanLogReader) GetScanHistories(_ context.Context, _ []string) (map[string][]bool, error) {
	return nil, f.err
}
func (f *fakeScanLogReader) GetJobsScannedToday(_ context.Context) (int64, error) {
	return f.scannedToday, f.err
}
func (f *fakeScanLogReader) GetScansByDay(_ context.Context, _ int) ([]dto.DailyScanCount, error) {
	return f.scansByDay, f.err
}
func (f *fakeScanLogReader) GetCompanyAnalytics(_ context.Context, _ string) (dto.CompanyAnalytics, error) {
	return dto.CompanyAnalytics{}, f.err
}

// fakeInsightsService is a test double for insights.Service.
type fakeInsightsService struct {
	pipeline insightsdto.PipelineCounts
	err      error
}

func (f *fakeInsightsService) TechStack(_ context.Context) ([]insightsdto.TechCount, error) {
	return nil, f.err
}
func (f *fakeInsightsService) CompanyActivity(_ context.Context) ([]insightsdto.CompanyActivityItem, error) {
	return nil, f.err
}
func (f *fakeInsightsService) Funnel(_ context.Context) ([]insightsdto.FunnelStep, error) {
	return nil, f.err
}
func (f *fakeInsightsService) Location(_ context.Context) (insightsdto.LocationBreakdown, error) {
	return insightsdto.LocationBreakdown{}, f.err
}
func (f *fakeInsightsService) Sponsorship(_ context.Context, _ insightsdto.InsightsFilter) ([]insightsdto.SponsorshipDay, error) {
	return nil, f.err
}
func (f *fakeInsightsService) Velocity(_ context.Context) ([]insightsdto.CompanyVelocity, error) {
	return nil, f.err
}
func (f *fakeInsightsService) RoleCategories(_ context.Context) ([]insightsdto.RoleCategoryCount, error) {
	return nil, f.err
}
func (f *fakeInsightsService) GetPipelineCounts(_ context.Context) (insightsdto.PipelineCounts, error) {
	return f.pipeline, f.err
}

func TestGetStats_OK(t *testing.T) {
	svc := services.NewDashboardImplService(
		&fakeScanLogReader{scannedToday: 10},
		&fakeInsightsService{pipeline: insightsdto.PipelineCounts{Pending: 3, Errors: 1}},
	)

	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.JobsScannedToday != 10 {
		t.Errorf("expected JobsScannedToday 10, got %d", stats.JobsScannedToday)
	}
	if stats.PipelinePending != 3 {
		t.Errorf("expected PipelinePending 3, got %d", stats.PipelinePending)
	}
	if stats.PipelineErrors != 1 {
		t.Errorf("expected PipelineErrors 1, got %d", stats.PipelineErrors)
	}
}

func TestGetStats_ScanLogError(t *testing.T) {
	svc := services.NewDashboardImplService(
		&fakeScanLogReader{err: errors.New("db error")},
		&fakeInsightsService{},
	)

	_, err := svc.GetStats(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

func TestGetStats_InsightsError(t *testing.T) {
	svc := services.NewDashboardImplService(
		&fakeScanLogReader{scannedToday: 10},
		&fakeInsightsService{err: errors.New("insights down")},
	)

	_, err := svc.GetStats(context.Background())
	if err == nil {
		t.Fatal("expected error from insights failure")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

// GetStats with all zero counts must produce a stats view with zero values.
func TestGetStats_AllZero(t *testing.T) {
	svc := services.NewDashboardImplService(
		&fakeScanLogReader{},
		&fakeInsightsService{},
	)

	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.JobsScannedToday != 0 {
		t.Errorf("unexpected non-zero JobsScannedToday: %d", stats.JobsScannedToday)
	}
	if stats.PipelinePending != 0 || stats.PipelineErrors != 0 {
		t.Errorf("unexpected non-zero pipeline counts: pending=%d errors=%d", stats.PipelinePending, stats.PipelineErrors)
	}
}

func TestGetActivity_OK(t *testing.T) {
	svc := services.NewDashboardImplService(
		&fakeScanLogReader{
			scansByDay: []dto.DailyScanCount{
				{Date: "2026-04-13", Scanned: 5, NewJobs: 3},
			},
		},
		&fakeInsightsService{},
	)

	activity, err := svc.GetActivity(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetActivity() error = %v", err)
	}
	if len(activity) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activity))
	}
	if activity[0].Date != "2026-04-13" {
		t.Errorf("expected date 2026-04-13, got %s", activity[0].Date)
	}
}

// GetActivity must surface DB errors as ErrInternal.
func TestGetActivity_Error(t *testing.T) {
	svc := services.NewDashboardImplService(
		&fakeScanLogReader{err: errors.New("scan log down")},
		&fakeInsightsService{},
	)

	_, err := svc.GetActivity(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error from scan log failure")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

// GetActivity with an empty result set must return a non-nil empty slice.
func TestGetActivity_EmptyResult(t *testing.T) {
	svc := services.NewDashboardImplService(
		&fakeScanLogReader{scansByDay: []dto.DailyScanCount{}},
		&fakeInsightsService{},
	)

	activity, err := svc.GetActivity(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activity == nil {
		t.Fatal("activity is nil; expected non-nil empty slice")
	}
	if len(activity) != 0 {
		t.Errorf("len = %d, want 0", len(activity))
	}
}

// GetActivity must map every scan row 1:1.
func TestGetActivity_MultipleDays(t *testing.T) {
	svc := services.NewDashboardImplService(
		&fakeScanLogReader{
			scansByDay: []dto.DailyScanCount{
				{Date: "2026-04-11", Scanned: 1, NewJobs: 0},
				{Date: "2026-04-12", Scanned: 5, NewJobs: 2},
				{Date: "2026-04-13", Scanned: 9, NewJobs: 4},
			},
		},
		&fakeInsightsService{},
	)

	activity, err := svc.GetActivity(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activity) != 3 {
		t.Fatalf("len = %d, want 3", len(activity))
	}
	if activity[2].Date != "2026-04-13" || activity[2].Scanned != 9 || activity[2].NewJobs != 4 {
		t.Errorf("row[2] = %+v, want {2026-04-13,9,4}", activity[2])
	}
}

// Ensure test doubles implement interfaces.
var _ scansvc.ScanLogReader = (*fakeScanLogReader)(nil)
var _ insightssvc.InsightsService = (*fakeInsightsService)(nil)
