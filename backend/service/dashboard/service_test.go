package dashboard

import (
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/service/insights"
	"github.com/trushilpatel/jobradar/service/scan"
)

// fakeScanLogReader is a test double for scan.ScanLogReader.
type fakeScanLogReader struct {
	scannedToday int64
	scansByDay   []scan.DailyScanCount
	err          error
}

func (f *fakeScanLogReader) GetScanStats(_ string) (scan.ScanStatsView, error) {
	return scan.ScanStatsView{}, f.err
}
func (f *fakeScanLogReader) GetJobsScannedToday() (int64, error) { return f.scannedToday, f.err }
func (f *fakeScanLogReader) GetScansByDay(_ int) ([]scan.DailyScanCount, error) {
	return f.scansByDay, f.err
}

// fakeInsightsService is a test double for insights.Service.
type fakeInsightsService struct {
	pipeline insights.PipelineCounts
	err      error
}

func (f *fakeInsightsService) TechStack() (interface{}, error)       { return nil, f.err }
func (f *fakeInsightsService) CompanyActivity() (interface{}, error) { return nil, f.err }
func (f *fakeInsightsService) Funnel() (interface{}, error)          { return nil, f.err }
func (f *fakeInsightsService) Location() (interface{}, error)        { return nil, f.err }
func (f *fakeInsightsService) Sponsorship(_ insights.InsightsFilter) (interface{}, error) {
	return nil, f.err
}
func (f *fakeInsightsService) Velocity() (interface{}, error)       { return nil, f.err }
func (f *fakeInsightsService) RoleCategories() (interface{}, error) { return nil, f.err }
func (f *fakeInsightsService) GetPipelineCounts() (insights.PipelineCounts, error) {
	return f.pipeline, f.err
}

// fakeJobCountReader is a test double for JobCountReader.
type fakeJobCountReader struct {
	newCount     int64
	savedCount   int64
	appliedCount int64
	err          error
}

func (f *fakeJobCountReader) GetJobCounts() (int64, int64, int64, error) {
	return f.newCount, f.savedCount, f.appliedCount, f.err
}

func TestGetStats_OK(t *testing.T) {
	svc := NewService(
		&fakeScanLogReader{scannedToday: 10},
		&fakeInsightsService{pipeline: insights.PipelineCounts{Pending: 3, Errors: 1}},
		&fakeJobCountReader{newCount: 5, savedCount: 2, appliedCount: 1},
	)

	stats, err := svc.GetStats()
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.JobsScannedToday != 10 {
		t.Errorf("expected JobsScannedToday 10, got %d", stats.JobsScannedToday)
	}
	if stats.JobsNew != 5 {
		t.Errorf("expected JobsNew 5, got %d", stats.JobsNew)
	}
	if stats.PipelinePending != 3 {
		t.Errorf("expected PipelinePending 3, got %d", stats.PipelinePending)
	}
	// Funnel relevant = new + saved + applied
	if stats.Funnel.Relevant != 8 {
		t.Errorf("expected Funnel.Relevant 8, got %d", stats.Funnel.Relevant)
	}
}

func TestGetStats_Error(t *testing.T) {
	svc := NewService(
		&fakeScanLogReader{err: errors.New("db error")},
		&fakeInsightsService{},
		&fakeJobCountReader{},
	)

	_, err := svc.GetStats()
	if err == nil {
		t.Fatal("expected error")
	}
	var sErr *response.ServiceError
	if !errors.As(err, &sErr) || sErr.Status != 500 {
		t.Errorf("expected 500 ServiceError, got %v", err)
	}
}

func TestGetActivity_OK(t *testing.T) {
	svc := NewService(
		&fakeScanLogReader{
			scansByDay: []scan.DailyScanCount{
				{Date: "2026-04-13", Scanned: 5, NewJobs: 3},
			},
		},
		&fakeInsightsService{},
		&fakeJobCountReader{},
	)

	activity, err := svc.GetActivity(7)
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

// Ensure test doubles implement interfaces.
var _ scan.ScanLogReader = (*fakeScanLogReader)(nil)
var _ insights.Service = (*fakeInsightsService)(nil)
var _ JobCountReader = (*fakeJobCountReader)(nil)
