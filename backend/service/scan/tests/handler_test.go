package scan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeScanService is a test double for Service.
type fakeScanService struct {
	queueResult []dto.QueueEntryView
	statsResult *dto.ScanStatsView
	err         error
}

func (f *fakeScanService) TriggerScan(_ context.Context, _ string) error { return f.err }
func (f *fakeScanService) TriggerScanAll(_ context.Context) (int, error) { return 0, f.err }
func (f *fakeScanService) GetStats(_ context.Context, _ string) (*dto.ScanStatsView, error) {
	return f.statsResult, f.err
}
func (f *fakeScanService) ListQueue(_ context.Context, _ dto.QueueFilter) ([]dto.QueueEntryView, error) {
	return f.queueResult, f.err
}
func (f *fakeScanService) RemoveFromQueue(_ context.Context, _ string) error { return f.err }
func (f *fakeScanService) EnqueueDue(_ context.Context, _ int) (svc.SchedulerCycleResult, error) {
	return svc.SchedulerCycleResult{}, f.err
}
func (f *fakeScanService) ResetStuck(_ context.Context, _ time.Duration) (int, error) {
	return 0, f.err
}
func (f *fakeScanService) PurgeInactive(_ context.Context) (int, error) { return 0, f.err }
func (f *fakeScanService) GetAnalytics(_ context.Context, _ string) (*dto.CompanyAnalytics, error) {
	return &dto.CompanyAnalytics{}, f.err
}

// newScanTestRouter wires the full decorator chain under test using the given
// service as the terminal, then mounts all routes onto a chi router.
func newScanTestRouter(next svc.Service) *chi.Mux {
	valid := svc.NewValidator(next)
	handler := svc.NewRestService(valid)
	router := svc.NewRestRouter(handler)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

func TestHandlerListQueue_OK(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{queueResult: []dto.QueueEntryView{}})

	req := httptest.NewRequest(http.MethodGet, "/companies/queue", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data"`) {
		t.Errorf("response missing data envelope: %s", w.Body.String())
	}
}

func TestHandlerListQueue_ValidationError(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{})

	req := httptest.NewRequest(http.MethodGet, "/companies/queue?status=invalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerTriggerScan_OK(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{})

	req := httptest.NewRequest(http.MethodPost, "/companies/stripe/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("got status %d, want 202", w.Code)
	}
}

func TestHandlerTriggerScan_NotFound(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{err: errors.ErrNotFound.WithMessage("company not found")})

	req := httptest.NewRequest(http.MethodPost, "/companies/missing/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestHandlerScanStats_OK(t *testing.T) {
	stats := &dto.ScanStatsView{TotalScans: 5}
	r := newScanTestRouter(&fakeScanService{statsResult: stats})

	req := httptest.NewRequest(http.MethodGet, "/companies/stripe/scan-stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerRemoveFromQueue_OK(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{})

	req := httptest.NewRequest(http.MethodDelete, "/companies/queue/entry-123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

// Ensure test double implements the interface.
var _ svc.Service = (*fakeScanService)(nil)
