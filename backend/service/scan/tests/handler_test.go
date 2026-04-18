package scan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// fakeScanValidator is a test double for Validator.
type fakeScanValidator struct {
	listErr    error
	triggerErr error
}

func (f *fakeScanValidator) ValidateListQueue(_ dto.QueueFilter) error { return f.listErr }
func (f *fakeScanValidator) ValidateTriggerScan(_ string) error        { return f.triggerErr }

// fakeScanService is a test double for Service.
type fakeScanService struct {
	queueResult []dto.QueueEntryView
	statsResult *dto.ScanStatsView
	err         error
}

func (f *fakeScanService) TriggerScan(_ context.Context, companyID string) error { return f.err }
func (f *fakeScanService) TriggerScanAll(_ context.Context) (int, error)         { return 0, f.err }
func (f *fakeScanService) GetStats(_ context.Context, companyID string) (*dto.ScanStatsView, error) {
	return f.statsResult, f.err
}
func (f *fakeScanService) ListQueue(_ context.Context, filter dto.QueueFilter) ([]dto.QueueEntryView, error) {
	return f.queueResult, f.err
}
func (f *fakeScanService) RemoveFromQueue(_ context.Context, entryID string) error { return f.err }
func (f *fakeScanService) MoveUp(_ context.Context, entryID string) error          { return f.err }
func (f *fakeScanService) MoveDown(_ context.Context, entryID string) error        { return f.err }
func (f *fakeScanService) EnqueueDue(_ context.Context, _ int) (int, error)        { return 0, f.err }
func (f *fakeScanService) ResetStuck(_ context.Context, _ time.Duration) error     { return f.err }

func newScanTestRouter(h *svc.RestService) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestHandlerListQueue_OK(t *testing.T) {
	h := svc.NewRestService(&fakeScanValidator{}, &fakeScanService{queueResult: []dto.QueueEntryView{}})
	r := newScanTestRouter(h)

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
	h := svc.NewRestService(
		&fakeScanValidator{listErr: &domain.ValidationError{Field: "status", Code: "INVALID_VALUE", Message: "bad"}},
		&fakeScanService{},
	)
	r := newScanTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/companies/queue?status=invalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerTriggerScan_OK(t *testing.T) {
	h := svc.NewRestService(&fakeScanValidator{}, &fakeScanService{})
	r := newScanTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/companies/stripe/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("got status %d, want 202", w.Code)
	}
}

func TestHandlerTriggerScan_NotFound(t *testing.T) {
	h := svc.NewRestService(
		&fakeScanValidator{},
		&fakeScanService{err: domain.ErrNotFound.WithMessage("company not found")},
	)
	r := newScanTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/companies/missing/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestHandlerScanStats_OK(t *testing.T) {
	stats := &dto.ScanStatsView{TotalScans: 5}
	h := svc.NewRestService(&fakeScanValidator{}, &fakeScanService{statsResult: stats})
	r := newScanTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/companies/stripe/scan-stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerRemoveFromQueue_OK(t *testing.T) {
	h := svc.NewRestService(&fakeScanValidator{}, &fakeScanService{})
	r := newScanTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/companies/queue/entry-123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

// Ensure test doubles implement interfaces.
var _ svc.Validator = (*fakeScanValidator)(nil)
var _ svc.Service = (*fakeScanService)(nil)
