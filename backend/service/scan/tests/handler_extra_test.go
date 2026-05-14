package scan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// These tests exercise the scan REST handlers' branches that are not yet
// covered by handler_test.go — TriggerScanAll, Analytics, and the
// remove/stats error paths.

func TestHandlerTriggerScanAll_OK(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{})

	req := httptest.NewRequest(http.MethodPost, "/companies/scan-all", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("got status %d, want 202; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"queued"`) {
		t.Errorf("response missing queued field: %s", w.Body.String())
	}
}

func TestHandlerTriggerScanAll_InternalError(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{err: apperr.ErrInternal.WithMessage("db down")})

	req := httptest.NewRequest(http.MethodPost, "/companies/scan-all", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestHandlerScanStats_InternalError(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{err: apperr.ErrInternal.WithMessage("db down")})

	req := httptest.NewRequest(http.MethodGet, "/companies/stripe/scan-stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestHandlerRemoveFromQueue_InternalError(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{err: apperr.ErrInternal.WithMessage("db down")})

	req := httptest.NewRequest(http.MethodDelete, "/companies/queue/entry-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestHandlerAnalytics_OK(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{})

	req := httptest.NewRequest(http.MethodGet, "/companies/stripe/analytics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerAnalytics_NotFound(t *testing.T) {
	r := newScanTestRouter(&fakeScanService{err: apperr.ErrNotFound.WithMessage("company not found")})

	req := httptest.NewRequest(http.MethodGet, "/companies/missing/analytics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

// TestHandlerListQueue_FilterPassThrough confirms the handler reads both
// status and source query params and forwards them — a regression here
// would silently drop filters and surface every queue entry.
func TestHandlerListQueue_FilterPassThrough(t *testing.T) {
	captured := &capturingScanService{}
	r := newScanTestRouter(captured)

	req := httptest.NewRequest(http.MethodGet, "/companies/queue?status=queued&source=manual", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if captured.lastFilter.Status != "queued" {
		t.Errorf("expected status=queued, got %q", captured.lastFilter.Status)
	}
	if captured.lastFilter.Source != "manual" {
		t.Errorf("expected source=manual, got %q", captured.lastFilter.Source)
	}
}

// capturingScanService records the QueueFilter passed by the handler so
// pass-through behaviour can be asserted without mocking the entire chain.
type capturingScanService struct {
	fakeScanService
	lastFilter dto.QueueFilter
}

func (c *capturingScanService) ListQueue(_ context.Context, f dto.QueueFilter) ([]dto.QueueEntryView, error) {
	c.lastFilter = f
	return nil, nil
}
