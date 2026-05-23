package dashboard_test

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
)

// recordingDashboardService captures method invocations so each BaseService
// delegation path can be asserted independently. Each method also returns an
// injectable error so the delegation contract is exercised through both
// success and failure paths.
type recordingDashboardService struct {
	statsCalled    bool
	stats          *dto.StatsView
	statsErr       error
	activityCalled bool
	activityDays   int
	activityRows   []dto.ActivityView
	activityErr    error
}

func (r *recordingDashboardService) GetStats(_ context.Context) (*dto.StatsView, error) {
	r.statsCalled = true
	return r.stats, r.statsErr
}
func (r *recordingDashboardService) GetActivity(_ context.Context, days int) ([]dto.ActivityView, error) {
	r.activityCalled = true
	r.activityDays = days
	return r.activityRows, r.activityErr
}

// baseDashboardWrapper exposes the unexported BaseService delegation methods
// without overriding them — exercising the embedded fields directly.
type baseDashboardWrapper struct {
	services.DashboardBaseService
}

func newBaseDashboardWrapper(next services.DashboardService) *baseDashboardWrapper {
	return &baseDashboardWrapper{DashboardBaseService: services.NewDashboardBaseService(next)}
}

// NewDashboardBaseService must panic when next is nil — a misconfigured chain
// is a programmer error, never a runtime condition the system should tolerate.
func TestDashboardBaseService_NilNextPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	services.NewDashboardBaseService(nil)
}

// Every BaseService method must call through to next and propagate both the
// argument and the return value (or error) unchanged.
func TestDashboardBaseService_DelegatesAll(t *testing.T) {
	rec := &recordingDashboardService{
		stats:        &dto.StatsView{JobsScannedToday: 42},
		activityRows: []dto.ActivityView{{Date: "2026-04-13", Scanned: 1, NewJobs: 0}},
	}
	svc := newBaseDashboardWrapper(rec)

	// GetStats
	out, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.statsCalled {
		t.Error("GetStats did not delegate to next")
	}
	if out == nil || out.JobsScannedToday != 42 {
		t.Errorf("stats = %+v, want JobsScannedToday=42", out)
	}

	// GetActivity — the days argument must propagate.
	rows, err := svc.GetActivity(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.activityCalled {
		t.Error("GetActivity did not delegate to next")
	}
	if rec.activityDays != 7 {
		t.Errorf("days = %d, want 7", rec.activityDays)
	}
	if len(rows) != 1 {
		t.Errorf("rows len = %d, want 1", len(rows))
	}
}

// BaseService delegation must propagate errors from next without wrapping.
func TestDashboardBaseService_PropagatesErrors(t *testing.T) {
	want := errors.New("downstream failure")
	rec := &recordingDashboardService{statsErr: want, activityErr: want}
	svc := newBaseDashboardWrapper(rec)

	if _, err := svc.GetStats(context.Background()); !errors.Is(err, want) {
		t.Errorf("GetStats err = %v, want propagated", err)
	}
	if _, err := svc.GetActivity(context.Background(), 5); !errors.Is(err, want) {
		t.Errorf("GetActivity err = %v, want propagated", err)
	}
}

// ---------- UserDashboardBaseService ----------

type recordingUserDashboardSvc struct {
	progressCalled bool
	marketCalled   bool
	heatmapCalled  bool
	interviews     bool
	velocity       bool
	trends         bool
	velocityLimit  int

	progressErr  error
	marketErr    error
	heatmapErr   error
	interviewErr error
	velocityErr  error
	trendsErr    error
}

func (r *recordingUserDashboardSvc) GetProgressStats(_ context.Context) (*dto.ProgressStats, error) {
	r.progressCalled = true
	return &dto.ProgressStats{}, r.progressErr
}
func (r *recordingUserDashboardSvc) GetMarketStats(_ context.Context) (*dto.MarketStats, error) {
	r.marketCalled = true
	return &dto.MarketStats{}, r.marketErr
}
func (r *recordingUserDashboardSvc) GetHeatmap(_ context.Context) ([]dto.WeeklyCount, error) {
	r.heatmapCalled = true
	return nil, r.heatmapErr
}
func (r *recordingUserDashboardSvc) GetInterviews(_ context.Context) ([]dto.InterviewRow, error) {
	r.interviews = true
	return nil, r.interviewErr
}
func (r *recordingUserDashboardSvc) GetVelocity(_ context.Context, limit int) ([]dto.CompanyVelocityRow, error) {
	r.velocity = true
	r.velocityLimit = limit
	return nil, r.velocityErr
}
func (r *recordingUserDashboardSvc) GetTechTrends(_ context.Context) (*dto.TechTrends, error) {
	r.trends = true
	return &dto.TechTrends{}, r.trendsErr
}

// baseUserDashboardWrapper exposes the embedded BaseService methods for direct
// invocation in tests.
type baseUserDashboardWrapper struct {
	services.UserDashboardBaseService
}

func TestUserDashboardBaseService_NilNextPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	services.NewUserDashboardBaseService(nil)
}

// Walk every UserDashboardBaseService method through to next.
func TestUserDashboardBaseService_DelegatesAll(t *testing.T) {
	rec := &recordingUserDashboardSvc{}
	svc := &baseUserDashboardWrapper{UserDashboardBaseService: services.NewUserDashboardBaseService(rec)}

	if _, err := svc.GetProgressStats(context.Background()); err != nil {
		t.Errorf("progress: %v", err)
	}
	if _, err := svc.GetMarketStats(context.Background()); err != nil {
		t.Errorf("market: %v", err)
	}
	if _, err := svc.GetHeatmap(context.Background()); err != nil {
		t.Errorf("heatmap: %v", err)
	}
	if _, err := svc.GetInterviews(context.Background()); err != nil {
		t.Errorf("interviews: %v", err)
	}
	if _, err := svc.GetVelocity(context.Background(), 11); err != nil {
		t.Errorf("velocity: %v", err)
	}
	if _, err := svc.GetTechTrends(context.Background()); err != nil {
		t.Errorf("trends: %v", err)
	}

	if !rec.progressCalled || !rec.marketCalled || !rec.heatmapCalled ||
		!rec.interviews || !rec.velocity || !rec.trends {
		t.Errorf("not all methods delegated: %+v", rec)
	}
	if rec.velocityLimit != 11 {
		t.Errorf("velocity limit = %d, want 11", rec.velocityLimit)
	}
}

// ---------- PipelineBaseService ----------

type recordingPipelineSvc struct {
	statsCalled bool
	retryCalled bool
	worker      string
	tier        string
	n           int64
	statsErr    error
	retryErr    error
}

func (r *recordingPipelineSvc) GetPipelineStats(_ context.Context) (*dto.PipelineStats, error) {
	r.statsCalled = true
	return &dto.PipelineStats{}, r.statsErr
}
func (r *recordingPipelineSvc) TriggerRetry(_ context.Context, worker, tier string) (int64, error) {
	r.retryCalled = true
	r.worker = worker
	r.tier = tier
	return r.n, r.retryErr
}

type basePipelineWrapper struct {
	services.PipelineBaseService
}

func TestPipelineBaseService_NilNextPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	services.NewPipelineBaseService(nil)
}

func TestPipelineBaseService_DelegatesAll(t *testing.T) {
	rec := &recordingPipelineSvc{n: 5}
	svc := &basePipelineWrapper{PipelineBaseService: services.NewPipelineBaseService(rec)}

	if _, err := svc.GetPipelineStats(context.Background()); err != nil {
		t.Fatalf("stats: %v", err)
	}
	if !rec.statsCalled {
		t.Error("GetPipelineStats did not delegate")
	}

	got, err := svc.TriggerRetry(context.Background(), "filter", "1m")
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !rec.retryCalled || rec.worker != "filter" || rec.tier != "1m" {
		t.Errorf("retry args not propagated: %+v", rec)
	}
	if got != 5 {
		t.Errorf("retry returned %d, want 5", got)
	}
}

// Ensure recording doubles implement contracts.
var _ services.DashboardService = (*recordingDashboardService)(nil)
var _ services.UserDashboardService = (*recordingUserDashboardSvc)(nil)
var _ services.PipelineService = (*recordingPipelineSvc)(nil)
