package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/domain"
)

// fakeDashboardValidator is a test double for Validator.
type fakeDashboardValidator struct {
	activityErr error
}

func (f *fakeDashboardValidator) ValidateActivity(_ int) error { return f.activityErr }

// fakeDashboardService is a test double for Service.
type fakeDashboardService struct {
	stats    *StatsView
	activity []ActivityView
	err      error
}

func (f *fakeDashboardService) GetStats(_ context.Context) (*StatsView, error) { return f.stats, f.err }
func (f *fakeDashboardService) GetActivity(_ context.Context, _ int) ([]ActivityView, error) {
	return f.activity, f.err
}

func newDashboardTestRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestDashboardHandlerStats_OK(t *testing.T) {
	h := NewHandler(&fakeDashboardValidator{}, &fakeDashboardService{stats: &StatsView{}}, nil, nil)
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
	h := NewHandler(
		&fakeDashboardValidator{},
		&fakeDashboardService{err: domain.ErrInternal.WithMessage("db down")},
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
	h := NewHandler(&fakeDashboardValidator{}, &fakeDashboardService{activity: []ActivityView{}}, nil, nil)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestDashboardHandlerActivity_ValidationError(t *testing.T) {
	h := NewHandler(
		&fakeDashboardValidator{activityErr: &domain.ValidationError{Field: "days", Code: "INVALID_VALUE", Message: "days must be >= 1"}},
		&fakeDashboardService{},
		nil, nil,
	)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestDashboardHandlerActivity_ServiceError(t *testing.T) {
	h := NewHandler(
		&fakeDashboardValidator{},
		&fakeDashboardService{err: domain.ErrInternal.WithMessage("db down")},
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
	stats *PipelineStats
	err   error
}

func (f *fakePipelineService) GetPipelineStats(_ context.Context) (*PipelineStats, error) {
	return f.stats, f.err
}

func TestDashboardHandlerPipeline_OK(t *testing.T) {
	h := NewHandler(
		&fakeDashboardValidator{},
		&fakeDashboardService{},
		nil,
		&fakePipelineService{stats: &PipelineStats{TotalCompanies: 5, ActiveCompanies: 4}},
	)
	r := newDashboardTestRouter(h)

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
	h := NewHandler(
		&fakeDashboardValidator{},
		&fakeDashboardService{},
		nil,
		&fakePipelineService{err: domain.ErrInternal.WithMessage("db down")},
	)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/pipeline", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// Ensure test doubles implement interfaces.
var _ Validator = (*fakeDashboardValidator)(nil)
var _ Service = (*fakeDashboardService)(nil)
var _ PipelineService = (*fakePipelineService)(nil)
