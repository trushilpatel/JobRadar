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
)

// fakeScanValidator is a test double for Validator.
type fakeScanValidator struct {
	listErr    error
	triggerErr error
}

func (f *fakeScanValidator) ValidateListQueue(_ QueueFilter) error { return f.listErr }
func (f *fakeScanValidator) ValidateTriggerScan(_ string) error    { return f.triggerErr }

// fakeScanService is a test double for Service.
type fakeScanService struct {
	queueResult []QueueEntryView
	statsResult *ScanStatsView
	err         error
}

func (f *fakeScanService) TriggerScan(_ context.Context, companyID string) error { return f.err }
func (f *fakeScanService) TriggerScanAll(_ context.Context) (int, error)         { return 0, f.err }
func (f *fakeScanService) GetStats(_ context.Context, companyID string) (*ScanStatsView, error) {
	return f.statsResult, f.err
}
func (f *fakeScanService) ListQueue(_ context.Context, filter QueueFilter) ([]QueueEntryView, error) {
	return f.queueResult, f.err
}
func (f *fakeScanService) RemoveFromQueue(_ context.Context, entryID string) error { return f.err }
func (f *fakeScanService) MoveUp(_ context.Context, entryID string) error          { return f.err }
func (f *fakeScanService) MoveDown(_ context.Context, entryID string) error        { return f.err }
func (f *fakeScanService) EnqueueDue(_ context.Context, _ int) (int, error)        { return 0, f.err }
func (f *fakeScanService) ResetStuck(_ context.Context, _ time.Duration) error     { return f.err }

func newScanTestRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestHandlerListQueue_OK(t *testing.T) {
	h := NewHandler(&fakeScanValidator{}, &fakeScanService{queueResult: []QueueEntryView{}})
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
	h := NewHandler(
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
	h := NewHandler(&fakeScanValidator{}, &fakeScanService{})
	r := newScanTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/companies/stripe/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("got status %d, want 202", w.Code)
	}
}

func TestHandlerTriggerScan_NotFound(t *testing.T) {
	h := NewHandler(
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
	stats := &ScanStatsView{TotalScans: 5}
	h := NewHandler(&fakeScanValidator{}, &fakeScanService{statsResult: stats})
	r := newScanTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/companies/stripe/scan-stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerRemoveFromQueue_OK(t *testing.T) {
	h := NewHandler(&fakeScanValidator{}, &fakeScanService{})
	r := newScanTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/companies/queue/entry-123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

// Ensure test doubles implement interfaces.
var _ Validator = (*fakeScanValidator)(nil)
var _ Service = (*fakeScanService)(nil)
