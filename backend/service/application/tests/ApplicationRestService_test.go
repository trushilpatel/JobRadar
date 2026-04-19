package application

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
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	svc "github.com/trushilpatel/jobradar/service/application/services"
)

// fakeValidator is a test double for ApplicationValidationService.
type fakeValidator struct {
	createErr      error
	updateStageErr error
	updateNoteErr  error
	listAllErr     error
}

func (f *fakeValidator) ValidateCreate(_ dto.CreateApplicationInput) error { return f.createErr }
func (f *fakeValidator) ValidateUpdateStage(_ dto.UpdateStageInput) error  { return f.updateStageErr }
func (f *fakeValidator) ValidateUpdateNote(_ dto.UpdateNoteInput) error    { return f.updateNoteErr }
func (f *fakeValidator) ValidateListAll(_, _ int) error                    { return f.listAllErr }

// fakeService is a test double for ApplicationService.
type fakeService struct {
	apps        []appmodel.JobApplication
	appsWithJob []svc.ApplicationWithJob
	app         *appmodel.JobApplication
	total       int
	err         error
}

func (f *fakeService) ListByJob(_ string) ([]appmodel.JobApplication, error) {
	return f.apps, f.err
}
func (f *fakeService) ListAll(_, _ int) ([]svc.ApplicationWithJob, int, error) {
	return f.appsWithJob, f.total, f.err
}
func (f *fakeService) Create(_ context.Context, _ string, _ dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	return f.app, f.err
}
func (f *fakeService) UpdateStage(_, _ string, _ dto.UpdateStageInput) error { return f.err }
func (f *fakeService) UpdateNote(_, _ string, _ dto.UpdateNoteInput) error   { return f.err }

func newTestRouter(h *svc.RestHandler) *chi.Mux {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestHandlerList_OK(t *testing.T) {
	h := svc.NewApplicationRestHandler(&fakeValidator{}, &fakeService{apps: []appmodel.JobApplication{}})
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/jobs/j1/applications", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerCreate_OK(t *testing.T) {
	app := &appmodel.JobApplication{Stage: appcommon.StageApplied}
	h := svc.NewApplicationRestHandler(&fakeValidator{}, &fakeService{app: app})
	r := newTestRouter(h)

	body, _ := json.Marshal(dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/applications", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("got status %d, want 201", w.Code)
	}
}

func TestHandlerCreate_NotFound(t *testing.T) {
	h := svc.NewApplicationRestHandler(
		&fakeValidator{},
		&fakeService{err: domain.ErrNotFound.WithMessage("job not found")},
	)
	r := newTestRouter(h)

	body, _ := json.Marshal(dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	req := httptest.NewRequest(http.MethodPost, "/jobs/missing/applications", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestHandlerListAll_OK(t *testing.T) {
	h := svc.NewApplicationRestHandler(&fakeValidator{}, &fakeService{appsWithJob: []svc.ApplicationWithJob{}})
	r := newTestRouter(h)

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

func TestHandlerUpdateStage_ValidationError(t *testing.T) {
	h := svc.NewApplicationRestHandler(
		&fakeValidator{updateStageErr: &domain.ValidationError{Field: "stage", Code: "INVALID_VALUE", Message: "bad"}},
		&fakeService{},
	)
	r := newTestRouter(h)

	body, _ := json.Marshal(dto.UpdateStageInput{Stage: "invalid"})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/stage", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerUpdateNote_OK(t *testing.T) {
	h := svc.NewApplicationRestHandler(&fakeValidator{}, &fakeService{})
	r := newTestRouter(h)

	body, _ := json.Marshal(dto.UpdateNoteInput{Note: "great call"})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/note", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

// Ensure test doubles implement interfaces.
var _ svc.ApplicationValidationService = (*fakeValidator)(nil)
var _ svc.ApplicationService = (*fakeService)(nil)
