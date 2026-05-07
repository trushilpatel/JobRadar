package dashboard_test

import (
	"context"
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
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{stats: &dto.StatsView{}}, nil, nil)
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
		nil, nil,
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
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{activity: []dto.ActivityView{}}, nil, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestDashboardHandlerActivity_ValidationError(t *testing.T) {
	// days=0 triggers the inlined validation in the handler (days < 1).
	h := dashboardsvc.NewDashboardRestService(&fakeDashboardService{}, nil, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestDashboardHandlerActivity_ServiceError(t *testing.T) {
	h := dashboardsvc.NewDashboardRestService(
		&fakeDashboardService{err: errors.ErrInternal.WithMessage("db down")},
		nil, nil,
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
}

func (f *fakePipelineService) GetPipelineStats(_ context.Context) (*dto.PipelineStats, error) {
	return f.stats, f.err
}

func (f *fakePipelineService) TriggerRetry(_ context.Context, _, _ string) (int64, error) {
	return 0, nil
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

// Ensure test doubles implement interfaces.
var _ dashboardsvc.DashboardService = (*fakeDashboardService)(nil)
var _ dashboardsvc.PipelineService = (*fakePipelineService)(nil)
