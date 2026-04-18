package job

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
)

// fakeJobValidator is a test double for Validator.
type fakeJobValidator struct {
	listErr    error
	dismissErr error
	statusErr  error
	notesErr   error
}

func (f *fakeJobValidator) ValidateList(_ JobFilter, _, _ int) error { return f.listErr }
func (f *fakeJobValidator) ValidateDismiss(_ DismissInput) error     { return f.dismissErr }
func (f *fakeJobValidator) ValidateSetStatus(_ string) error         { return f.statusErr }
func (f *fakeJobValidator) ValidateSetNotes(_ SetNotesInput) error   { return f.notesErr }

// fakeJobService is a test double for Service.
type fakeJobService struct {
	jobs  []model.Job
	job   *model.Job
	total int
	err   error
}

func (f *fakeJobService) List(_ context.Context, _ JobFilter, _, _ int) ([]model.Job, int, error) {
	return f.jobs, f.total, f.err
}
func (f *fakeJobService) Get(_ context.Context, _ string) (*model.Job, error)       { return f.job, f.err }
func (f *fakeJobService) GetCounts(_ context.Context) (*JobCounts, error)           { return &JobCounts{}, f.err }
func (f *fakeJobService) Dismiss(_ context.Context, _ string, _ DismissInput) error { return f.err }
func (f *fakeJobService) SetSaved(_ context.Context, _ string, _ bool) error        { return f.err }
func (f *fakeJobService) SetStatus(_ context.Context, _, _ string) error            { return f.err }
func (f *fakeJobService) SetNotes(_ context.Context, _, _ string) error             { return f.err }

func newJobTestRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestJobHandlerList_OK(t *testing.T) {
	h := NewHandler(&fakeJobValidator{}, &fakeJobService{jobs: []model.Job{}, total: 0})
	r := newJobTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"data"`) {
		t.Errorf("body missing data envelope: %s", body)
	}
	if !strings.Contains(body, `"total"`) {
		t.Errorf("body missing total: %s", body)
	}
}

func TestJobHandlerList_ValidationError(t *testing.T) {
	h := NewHandler(
		&fakeJobValidator{listErr: &domain.ValidationError{Field: "status", Code: "INVALID_VALUE", Message: "bad"}},
		&fakeJobService{},
	)
	r := newJobTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs?status=dismissed", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestJobHandlerGet_OK(t *testing.T) {
	h := NewHandler(&fakeJobValidator{}, &fakeJobService{job: &model.Job{ID: "j1"}})
	r := newJobTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs/j1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestJobHandlerGet_NotFound(t *testing.T) {
	h := NewHandler(
		&fakeJobValidator{},
		&fakeJobService{err: domain.ErrNotFound.WithMessage("job not found")},
	)
	r := newJobTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestJobHandlerDismiss_OK(t *testing.T) {
	h := NewHandler(&fakeJobValidator{}, &fakeJobService{})
	r := newJobTestRouter(h)

	body, _ := json.Marshal(DismissInput{Reason: model.DiscardReasonDismissed})
	req := httptest.NewRequest(http.MethodDelete, "/jobs/j1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestJobHandlerDismiss_ValidationError(t *testing.T) {
	h := NewHandler(
		&fakeJobValidator{dismissErr: &domain.ValidationError{Field: "reason", Code: "INVALID_VALUE", Message: "bad reason"}},
		&fakeJobService{},
	)
	r := newJobTestRouter(h)

	body, _ := json.Marshal(DismissInput{Reason: "invalid"})
	req := httptest.NewRequest(http.MethodDelete, "/jobs/j1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestJobHandlerSetStatus_OK(t *testing.T) {
	h := NewHandler(&fakeJobValidator{}, &fakeJobService{})
	r := newJobTestRouter(h)

	body, _ := json.Marshal(SetStatusInput{Status: model.JobStatusApplied})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/status", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestJobHandlerSetNotes_OK(t *testing.T) {
	h := NewHandler(&fakeJobValidator{}, &fakeJobService{})
	r := newJobTestRouter(h)

	body, _ := json.Marshal(SetNotesInput{Notes: "great company"})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/notes", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestJobHandlerGetCounts_OK(t *testing.T) {
	h := NewHandler(&fakeJobValidator{}, &fakeJobService{})
	r := newJobTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs/counts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"data"`) {
		t.Errorf("body missing data envelope: %s", body)
	}
}

// capturingValidator records the filter passed to ValidateList for inspection.
type capturingValidator struct {
	filter JobFilter
}

func (c *capturingValidator) ValidateList(f JobFilter, _, _ int) error {
	c.filter = f
	return nil
}
func (c *capturingValidator) ValidateDismiss(_ DismissInput) error   { return nil }
func (c *capturingValidator) ValidateSetStatus(_ string) error       { return nil }
func (c *capturingValidator) ValidateSetNotes(_ SetNotesInput) error { return nil }

func TestJobHandlerList_ExperienceRangesParsed(t *testing.T) {
	capturing := &capturingValidator{}
	h := NewHandler(capturing, &fakeJobService{})
	r := newJobTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs?experience_ranges=0-2&experience_ranges=5%2B", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if len(capturing.filter.ExperienceRanges) != 2 {
		t.Fatalf("expected 2 parsed ranges, got %d", len(capturing.filter.ExperienceRanges))
	}
	r0 := capturing.filter.ExperienceRanges[0]
	if r0.Min != 0 || r0.Max == nil || *r0.Max != 2 {
		t.Errorf("range[0] = {Min:%d Max:%v}, want {0 2}", r0.Min, r0.Max)
	}
	r1 := capturing.filter.ExperienceRanges[1]
	if r1.Min != 5 || r1.Max != nil {
		t.Errorf("range[1] = {Min:%d Max:%v}, want {5 nil}", r1.Min, r1.Max)
	}
}

func TestJobHandlerList_ExperienceRangesInvalidDropped(t *testing.T) {
	capturing := &capturingValidator{}
	h := NewHandler(capturing, &fakeJobService{})
	r := newJobTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs?experience_ranges=bad&experience_ranges=2-5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if len(capturing.filter.ExperienceRanges) != 1 {
		t.Fatalf("expected 1 range (invalid dropped), got %d", len(capturing.filter.ExperienceRanges))
	}
	r0 := capturing.filter.ExperienceRanges[0]
	if r0.Min != 2 || r0.Max == nil || *r0.Max != 5 {
		t.Errorf("range[0] = {Min:%d Max:%v}, want {2 5}", r0.Min, r0.Max)
	}
}

// Ensure test doubles implement interfaces.
var _ Validator = (*fakeJobValidator)(nil)
var _ Validator = (*capturingValidator)(nil)
var _ Service = (*fakeJobService)(nil)
