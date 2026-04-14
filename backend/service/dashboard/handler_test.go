package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/response"
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

func (f *fakeDashboardService) GetStats() (*StatsView, error)             { return f.stats, f.err }
func (f *fakeDashboardService) GetActivity(_ int) ([]ActivityView, error) { return f.activity, f.err }

func newDashboardTestRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestDashboardHandlerStats_OK(t *testing.T) {
	h := NewHandler(&fakeDashboardValidator{}, &fakeDashboardService{stats: &StatsView{}})
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
		&fakeDashboardService{err: response.ErrInternal.WithMessage("db down")},
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
	h := NewHandler(&fakeDashboardValidator{}, &fakeDashboardService{activity: []ActivityView{}})
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
		&fakeDashboardValidator{activityErr: &response.ValidationError{Field: "days", Code: "INVALID_VALUE", Message: "days must be >= 1"}},
		&fakeDashboardService{},
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
		&fakeDashboardService{err: response.ErrInternal.WithMessage("db down")},
	)
	r := newDashboardTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// Ensure test doubles implement interfaces.
var _ Validator = (*fakeDashboardValidator)(nil)
var _ Service = (*fakeDashboardService)(nil)
