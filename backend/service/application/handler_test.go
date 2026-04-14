package application

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

// fakeAppValidator is a test double for Validator.
type fakeAppValidator struct {
	createErr      error
	updateStageErr error
	updateNoteErr  error
	listAllErr     error
}

func (f *fakeAppValidator) ValidateCreate(_ CreateApplicationInput) error    { return f.createErr }
func (f *fakeAppValidator) ValidateUpdateStage(_ UpdateStageInput) error     { return f.updateStageErr }
func (f *fakeAppValidator) ValidateUpdateNote(_ UpdateNoteInput) error       { return f.updateNoteErr }
func (f *fakeAppValidator) ValidateListAll(_ request.Pagination) error       { return f.listAllErr }

// fakeAppService is a test double for Service.
type fakeAppService struct {
	apps        []model.JobApplication
	appsWithJob []ApplicationWithJob
	app         *model.JobApplication
	total       int
	err         error
}

func (f *fakeAppService) ListByJob(_ string) ([]model.JobApplication, error) {
	return f.apps, f.err
}
func (f *fakeAppService) ListAll(_ request.Pagination) ([]ApplicationWithJob, int, error) {
	return f.appsWithJob, f.total, f.err
}
func (f *fakeAppService) Create(_ string, _ CreateApplicationInput) (*model.JobApplication, error) {
	return f.app, f.err
}
func (f *fakeAppService) UpdateStage(_, _ string, _ UpdateStageInput) error { return f.err }
func (f *fakeAppService) UpdateNote(_, _ string, _ UpdateNoteInput) error   { return f.err }

func newAppTestRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestAppHandlerList_OK(t *testing.T) {
	h := NewHandler(&fakeAppValidator{}, &fakeAppService{apps: []model.JobApplication{}})
	r := newAppTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs/j1/applications", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestAppHandlerCreate_OK(t *testing.T) {
	app := &model.JobApplication{Stage: model.StageApplied}
	h := NewHandler(&fakeAppValidator{}, &fakeAppService{app: app})
	r := newAppTestRouter(h)

	body, _ := json.Marshal(CreateApplicationInput{Stage: model.StageApplied})
	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/applications", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("got status %d, want 201", w.Code)
	}
}

func TestAppHandlerCreate_NotFound(t *testing.T) {
	h := NewHandler(
		&fakeAppValidator{},
		&fakeAppService{err: response.ErrNotFound.WithMessage("job not found")},
	)
	r := newAppTestRouter(h)

	body, _ := json.Marshal(CreateApplicationInput{Stage: model.StageApplied})
	req := httptest.NewRequest(http.MethodPost, "/jobs/missing/applications", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestAppHandlerListAll_OK(t *testing.T) {
	h := NewHandler(&fakeAppValidator{}, &fakeAppService{appsWithJob: []ApplicationWithJob{}})
	r := newAppTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/applications", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"total"`) {
		t.Errorf("body missing total: %s", w.Body.String())
	}
}

func TestAppHandlerUpdateStage_ValidationError(t *testing.T) {
	h := NewHandler(
		&fakeAppValidator{updateStageErr: &response.ValidationError{Field: "stage", Code: "INVALID_VALUE", Message: "bad"}},
		&fakeAppService{},
	)
	r := newAppTestRouter(h)

	body, _ := json.Marshal(UpdateStageInput{Stage: "invalid"})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/stage", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestAppHandlerUpdateNote_OK(t *testing.T) {
	h := NewHandler(&fakeAppValidator{}, &fakeAppService{})
	r := newAppTestRouter(h)

	body, _ := json.Marshal(UpdateNoteInput{Note: "great call"})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/note", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

// Ensure test doubles implement interfaces.
var _ Validator = (*fakeAppValidator)(nil)
var _ Service = (*fakeAppService)(nil)
