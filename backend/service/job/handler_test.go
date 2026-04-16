package job

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
)

// fakeJobValidator is a test double for Validator.
type fakeJobValidator struct {
	listErr    error
	dismissErr error
	statusErr  error
	notesErr   error
}

func (f *fakeJobValidator) ValidateList(_ JobFilter, _ request.Pagination) error { return f.listErr }
func (f *fakeJobValidator) ValidateDismiss(_ DismissInput) error                 { return f.dismissErr }
func (f *fakeJobValidator) ValidateSetStatus(_ string) error                     { return f.statusErr }
func (f *fakeJobValidator) ValidateSetNotes(_ SetNotesInput) error               { return f.notesErr }

// fakeJobService is a test double for Service.
type fakeJobService struct {
	jobs  []model.Job
	job   *model.Job
	total int
	err   error
}

func (f *fakeJobService) List(_ JobFilter, _ request.Pagination) ([]model.Job, int, error) {
	return f.jobs, f.total, f.err
}
func (f *fakeJobService) Get(_ string) (*model.Job, error)       { return f.job, f.err }
func (f *fakeJobService) GetCounts() (*JobCounts, error)         { return &JobCounts{}, f.err }
func (f *fakeJobService) Dismiss(_ string, _ DismissInput) error { return f.err }
func (f *fakeJobService) SetSaved(_ string, _ bool) error        { return f.err }
func (f *fakeJobService) SetStatus(_, _ string) error            { return f.err }
func (f *fakeJobService) SetNotes(_, _ string) error             { return f.err }

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
		&fakeJobValidator{listErr: &response.ValidationError{Field: "status", Code: "INVALID_VALUE", Message: "bad"}},
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
		&fakeJobService{err: response.ErrNotFound.WithMessage("job not found")},
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
		&fakeJobValidator{dismissErr: &response.ValidationError{Field: "reason", Code: "INVALID_VALUE", Message: "bad reason"}},
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

// Ensure test doubles implement interfaces.
var _ Validator = (*fakeJobValidator)(nil)
var _ Service = (*fakeJobService)(nil)
