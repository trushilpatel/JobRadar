package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
)

// fakeDashboardService is a test double for DashboardService.
type fakeDashboardService struct {
	stats    *dto.StatsView
	activity []dto.ActivityView
	err      error
}

func (f *fakeDashboardService) GetStats(_ context.Context) (*dto.StatsView, error) {
	return f.stats, f.err
}
func (f *fakeDashboardService) GetActivity(_ context.Context, _ int) ([]dto.ActivityView, error) {
	return f.activity, f.err
}

func newDashboardTestRouter(h *dashboardsvc.RestService) *chi.Mux {
	r := chi.NewRouter()
	router := dashboardsvc.NewRestRouter(h)
	router.Register(r)
	return r
}

func TestDashboardHandlerStats_OK(t *testing.T) {
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{stats: &dto.StatsView{}}, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("body missing data envelope: %s", w.Body.String())
	}
}

func TestDashboardHandlerStats_ServiceError(t *testing.T) {
	h := dashboardsvc.NewDashboardRestService(
		&fakeDashboardService{err: errors.ErrInternal.WithMessage("db down")},
		nil,
	)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"error"`) {
		t.Errorf("body missing error envelope: %s", w.Body.String())
	}
}

func TestDashboardHandlerActivity_OK(t *testing.T) {
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{activity: []dto.ActivityView{}}, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestDashboardHandlerActivity_ValidationError(t *testing.T) {
	// days=0 triggers the validation: days must be between 1 and 365.
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{}, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

// days=abc must return 400 INVALID_FORMAT (non-integer).
func TestDashboardHandlerActivity_InvalidFormat(t *testing.T) {
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{}, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

// days=366 must return 400 INVALID_VALUE (exceeds upper bound).
func TestDashboardHandlerActivity_ExceedsUpperBound(t *testing.T) {
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{}, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=366", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

// days=365 must be accepted (boundary value).
func TestDashboardHandlerActivity_MaxBoundary(t *testing.T) {
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{activity: []dto.ActivityView{}}, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=365", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestDashboardHandlerActivity_ServiceError(t *testing.T) {
	h := dashboardsvc.NewDashboardRestService(
		&fakeDashboardService{err: errors.ErrInternal.WithMessage("db down")},
		nil,
	)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// fakePipelineService is a test double for PipelineService.
type fakePipelineService struct {
	stats *dto.PipelineStats
	err   error

	triggerWorker string
	triggerTier   string
	triggerN      int64
	triggerErr    error
}

func (f *fakePipelineService) GetPipelineStats(_ context.Context) (*dto.PipelineStats, error) {
	return f.stats, f.err
}

func (f *fakePipelineService) TriggerRetry(_ context.Context, worker, tier string) (int64, error) {
	f.triggerWorker = worker
	f.triggerTier = tier
	return f.triggerN, f.triggerErr
}

// Phase 2: /dashboard/pipeline moved to /admin/dashboard/pipeline (PipelineRestRouter).
// The following tests use PipelineRestService + PipelineRestRouter directly.

func newPipelineTestRouter(svc dashboardsvc.PipelineService) *chi.Mux {
	r := chi.NewRouter()
	rest := dashboardsvc.NewPipelineRestService(svc)
	router := dashboardsvc.NewPipelineRestRouter(rest)
	router.Register(r)
	return r
}

func TestDashboardHandlerPipeline_OK(t *testing.T) {
	r := newPipelineTestRouter(&fakePipelineService{
		stats: &dto.PipelineStats{TotalCompanies: 5, ActiveCompanies: 4},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/pipeline", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("body missing data envelope: %s", w.Body.String())
	}
}

func TestDashboardHandlerPipeline_ServiceError(t *testing.T) {
	r := newPipelineTestRouter(&fakePipelineService{
		err: errors.ErrInternal.WithMessage("db down"),
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/pipeline", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// fakeUserDashboardSvc is a test double for the UserDashboardService interface.
type fakeUserDashboardSvc struct {
	progress     *dto.ProgressStats
	market       *dto.MarketStats
	heatmap      []dto.WeeklyCount
	interviews   []dto.InterviewRow
	velocity     []dto.CompanyVelocityRow
	techTrends   *dto.TechTrends
	velocityArg  int
	progressErr  error
	marketErr    error
	heatmapErr   error
	interviewErr error
	velocityErr  error
	trendsErr    error
}

func (f *fakeUserDashboardSvc) GetProgressStats(_ context.Context) (*dto.ProgressStats, error) {
	return f.progress, f.progressErr
}
func (f *fakeUserDashboardSvc) GetMarketStats(_ context.Context) (*dto.MarketStats, error) {
	return f.market, f.marketErr
}
func (f *fakeUserDashboardSvc) GetHeatmap(_ context.Context) ([]dto.WeeklyCount, error) {
	return f.heatmap, f.heatmapErr
}
func (f *fakeUserDashboardSvc) GetInterviews(_ context.Context) ([]dto.InterviewRow, error) {
	return f.interviews, f.interviewErr
}
func (f *fakeUserDashboardSvc) GetVelocity(_ context.Context, limit int) ([]dto.CompanyVelocityRow, error) {
	f.velocityArg = limit
	return f.velocity, f.velocityErr
}
func (f *fakeUserDashboardSvc) GetTechTrends(_ context.Context) (*dto.TechTrends, error) {
	return f.techTrends, f.trendsErr
}

// newUserDashboardRouter mounts the user-facing dashboard router.
func newUserDashboardRouter(u *fakeUserDashboardSvc) *chi.Mux {
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{}, u)
	return newDashboardTestRouter(h)
}

// ---------- Progress ----------

func TestDashboardHandlerProgress_OK(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{progress: &dto.ProgressStats{AppliedCount: 3}})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/progress", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"applied_count":3`) {
		t.Errorf("body missing applied_count: %s", w.Body.String())
	}
}

func TestDashboardHandlerProgress_ServiceError(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{progressErr: errors.ErrInternal.WithMessage("db down")})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/progress", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ---------- Market ----------

func TestDashboardHandlerMarket_OK(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{market: &dto.MarketStats{TotalOpen: 42}})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/market", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"total_open":42`) {
		t.Errorf("body missing total_open: %s", w.Body.String())
	}
}

func TestDashboardHandlerMarket_ServiceError(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{marketErr: errors.ErrInternal.WithMessage("db down")})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/market", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ---------- Heatmap ----------

func TestDashboardHandlerHeatmap_OK(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{heatmap: []dto.WeeklyCount{{WeekStart: "2026-04-06", JobCount: 9}}})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/heatmap", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"job_count":9`) {
		t.Errorf("body missing job_count: %s", w.Body.String())
	}
}

func TestDashboardHandlerHeatmap_ServiceError(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{heatmapErr: errors.ErrInternal.WithMessage("db down")})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/heatmap", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ---------- Interviews ----------

func TestDashboardHandlerInterviews_OK(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{interviews: []dto.InterviewRow{{ApplicationID: 1, JobTitle: "SWE"}}})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/interviews", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"job_title":"SWE"`) {
		t.Errorf("body missing job_title: %s", w.Body.String())
	}
}

func TestDashboardHandlerInterviews_ServiceError(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{interviewErr: errors.ErrInternal.WithMessage("db down")})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/interviews", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ---------- Velocity ----------

// Default limit (no query string) must be 10.
func TestDashboardHandlerVelocity_DefaultLimit(t *testing.T) {
	svc := &fakeUserDashboardSvc{velocity: []dto.CompanyVelocityRow{{CompanyName: "Acme"}}}
	r := newUserDashboardRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/velocity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if svc.velocityArg != 10 {
		t.Errorf("default limit passed = %d, want 10", svc.velocityArg)
	}
}

// limit=25 in query string must be parsed and propagated.
func TestDashboardHandlerVelocity_CustomLimit(t *testing.T) {
	svc := &fakeUserDashboardSvc{velocity: []dto.CompanyVelocityRow{}}
	r := newUserDashboardRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/velocity?limit=25", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if svc.velocityArg != 25 {
		t.Errorf("limit = %d, want 25", svc.velocityArg)
	}
}

// limit=abc must return 400 — non-integer input is rejected.
func TestDashboardHandlerVelocity_InvalidLimit_Returns400(t *testing.T) {
	svc := &fakeUserDashboardSvc{velocity: []dto.CompanyVelocityRow{}}
	r := newUserDashboardRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/velocity?limit=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// limit=0 must return 400 — out-of-range (must be 1-100).
func TestDashboardHandlerVelocity_ZeroLimit_Returns400(t *testing.T) {
	svc := &fakeUserDashboardSvc{velocity: []dto.CompanyVelocityRow{}}
	r := newUserDashboardRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/velocity?limit=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// limit=101 must return 400 — exceeds upper bound.
func TestDashboardHandlerVelocity_ExceedsUpperBound_Returns400(t *testing.T) {
	svc := &fakeUserDashboardSvc{velocity: []dto.CompanyVelocityRow{}}
	r := newUserDashboardRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/velocity?limit=101", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// limit=100 must be accepted (boundary value).
func TestDashboardHandlerVelocity_MaxBoundary(t *testing.T) {
	svc := &fakeUserDashboardSvc{velocity: []dto.CompanyVelocityRow{}}
	r := newUserDashboardRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/velocity?limit=100", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if svc.velocityArg != 100 {
		t.Errorf("limit = %d, want 100", svc.velocityArg)
	}
}

func TestDashboardHandlerVelocity_ServiceError(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{velocityErr: errors.ErrInternal.WithMessage("db down")})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/velocity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ---------- TechTrends ----------

func TestDashboardHandlerTechTrends_OK(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{techTrends: &dto.TechTrends{TotalJobs: 50}})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/tech-trends", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"total_jobs":50`) {
		t.Errorf("body missing total_jobs: %s", w.Body.String())
	}
}

func TestDashboardHandlerTechTrends_ServiceError(t *testing.T) {
	r := newUserDashboardRouter(&fakeUserDashboardSvc{trendsErr: errors.ErrInternal.WithMessage("db down")})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/tech-trends", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ---------- Pipeline TriggerRetry ----------

// Happy path — well-formed body, drainer returns a positive count.
func TestPipelineHandlerTriggerRetry_OK(t *testing.T) {
	svc := &fakePipelineService{triggerN: 17}
	r := newPipelineTestRouter(svc)

	body := []byte(`{"worker":"filter","tier":"10m"}`)
	req := httptest.NewRequest(http.MethodPost, "/dashboard/pipeline/retry", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if svc.triggerWorker != "filter" || svc.triggerTier != "10m" {
		t.Errorf("worker/tier captured = %q/%q, want filter/10m", svc.triggerWorker, svc.triggerTier)
	}

	var resp struct {
		Data dto.TriggerRetryResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if resp.Data.Drained != 17 {
		t.Errorf("drained = %d, want 17", resp.Data.Drained)
	}
}

// Malformed JSON body must surface as 400 BAD_REQUEST.
func TestPipelineHandlerTriggerRetry_MalformedBody(t *testing.T) {
	r := newPipelineTestRouter(&fakePipelineService{})
	req := httptest.NewRequest(http.MethodPost, "/dashboard/pipeline/retry", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// Underlying service error must propagate the service status code.
func TestPipelineHandlerTriggerRetry_ServiceError(t *testing.T) {
	r := newPipelineTestRouter(&fakePipelineService{triggerErr: errors.ErrBadRequest.WithMessage("unknown worker")})
	body := []byte(`{"worker":"unknown","tier":"1m"}`)
	req := httptest.NewRequest(http.MethodPost, "/dashboard/pipeline/retry", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// NewPipelineRestService must panic when next is nil.
func TestPipelineRestService_NilNextPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	dashboardsvc.NewPipelineRestService(nil)
}

// Ensure test doubles implement interfaces.
var _ dashboardsvc.DashboardService = (*fakeDashboardService)(nil)
var _ dashboardsvc.PipelineService = (*fakePipelineService)(nil)
var _ dashboardsvc.UserDashboardService = (*fakeUserDashboardSvc)(nil)
