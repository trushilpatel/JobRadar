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
	"github.com/trushilpatel/jobradar/common/errors"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	svc "github.com/trushilpatel/jobradar/service/application/services"
)

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
func (f *fakeService) ListActive() ([]svc.ApplicationWithJob, error) {
	return f.appsWithJob, f.err
}
func (f *fakeService) Create(_ context.Context, _ string, _ dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	return f.app, f.err
}
func (f *fakeService) UpdateStage(_, _ string, _ dto.UpdateStageInput) error { return f.err }
func (f *fakeService) UpdateNote(_, _ string, _ dto.UpdateNoteInput) error   { return f.err }

// newTestRouter wires a RestService directly (bypassing validation) and registers routes via RestRouter.
func newTestRouter(next svc.ApplicationService) *chi.Mux {
	r := chi.NewRouter()
	h := svc.NewApplicationRestService(next)
	rt := svc.NewApplicationRestRouter(h)
	rt.Register(r)
	return r
}

func TestHandlerList_OK(t *testing.T) {
	r := newTestRouter(&fakeService{apps: []appmodel.JobApplication{}})

	req := httptest.NewRequest(http.MethodGet, "/jobs/j1/applications", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerCreate_OK(t *testing.T) {
	app := &appmodel.JobApplication{Stage: appcommon.StageApplied}
	r := newTestRouter(&fakeService{app: app})

	body, _ := json.Marshal(dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/applications", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("got status %d, want 201", w.Code)
	}
}

func TestHandlerCreate_NotFound(t *testing.T) {
	r := newTestRouter(&fakeService{err: errors.ErrNotFound.WithMessage("job not found")})

	body, _ := json.Marshal(dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	req := httptest.NewRequest(http.MethodPost, "/jobs/missing/applications", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestHandlerListAll_OK(t *testing.T) {
	r := newTestRouter(&fakeService{appsWithJob: []svc.ApplicationWithJob{}})

	req := httptest.NewRequest(http.MethodGet, "/applications?page=1&limit=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"total"`) {
		t.Errorf("body missing total: %s", w.Body.String())
	}
}

func TestHandlerUpdateStage_OK(t *testing.T) {
	r := newTestRouter(&fakeService{})

	body, _ := json.Marshal(dto.UpdateStageInput{Stage: appcommon.StageApplied})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/stage", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

func TestHandlerUpdateNote_OK(t *testing.T) {
	r := newTestRouter(&fakeService{})

	body, _ := json.Marshal(dto.UpdateNoteInput{Note: "great call"})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/note", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
}

// Ensure test double implements interface.
var _ svc.ApplicationService = (*fakeService)(nil)
