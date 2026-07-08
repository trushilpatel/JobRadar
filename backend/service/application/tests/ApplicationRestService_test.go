package application

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	svc "github.com/trushilpatel/jobradar/service/application/services"
)

// fakeService is a test double for ApplicationService used to drive the REST handler.
type fakeService struct {
	apps        []appmodel.JobApplication
	appsWithJob []svc.ApplicationWithJob
	app         *appmodel.JobApplication
	total       int
	err         error
}

func (f *fakeService) ListByJob(_ context.Context, _ string) ([]appmodel.JobApplication, error) {
	return f.apps, f.err
}
func (f *fakeService) ListAll(_ context.Context, _, _ int) ([]svc.ApplicationWithJob, int, error) {
	return f.appsWithJob, f.total, f.err
}
func (f *fakeService) ListActive(_ context.Context) ([]svc.ApplicationWithJob, error) {
	return f.appsWithJob, f.err
}
func (f *fakeService) Create(_ context.Context, _ string, _ dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	return f.app, f.err
}
func (f *fakeService) UpdateStage(_ context.Context, _, _ string, _ dto.UpdateStageInput) error {
	return f.err
}
func (f *fakeService) UpdateStageByID(_ context.Context, _ string, _ dto.UpdateStageInput) error {
	return f.err
}
func (f *fakeService) UpdateNote(_ context.Context, _, _ string, _ dto.UpdateNoteInput) error {
	return f.err
}

// newTestRouter wires a RestService directly (bypassing validation) and
// registers routes via RestRouter — tests below rely on the relative routes
// declared by ApplicationRestRouter.
func newTestZoneConfig() huma.Config {
	return apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
}

func newTestRouter(next svc.ApplicationService) *chi.Mux {
	r := chi.NewRouter()
	h := svc.NewRestService(next)
	rt := svc.NewRestRouter(h, newTestZoneConfig())
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
	r := newTestRouter(&fakeService{err: apperr.ErrNotFound.WithMessage("job not found")})

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

func TestHandlerUpdateStageByID_OK(t *testing.T) {
	r := newTestRouter(&fakeService{})

	body, _ := json.Marshal(dto.UpdateStageInput{Stage: appcommon.StageApplied})
	req := httptest.NewRequest(http.MethodPatch, "/applications/1/stage", bytes.NewReader(body))
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

// TestHandlerList_Error confirms ListByJob's error path routes through
// HandleError → ServiceError status mapping.
func TestHandlerList_Error(t *testing.T) {
	r := newTestRouter(&fakeService{err: apperr.ErrInternal.WithMessage("boom")})

	req := httptest.NewRequest(http.MethodGet, "/jobs/j1/applications", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// TestHandlerCreate_BadJSON exercises the json decode failure branch — a
// malformed body must yield 400 BAD_REQUEST without ever invoking the chain.
func TestHandlerCreate_BadJSON(t *testing.T) {
	called := false
	fake := &fakeServiceWithFlag{flag: &called}
	r := chi.NewRouter()
	h := svc.NewRestService(fake)
	rt := svc.NewRestRouter(h, newTestZoneConfig())
	rt.Register(r)

	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/applications", bytes.NewReader([]byte("{not-json")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
	if called {
		t.Error("next service must not be called on malformed body")
	}
	if !strings.Contains(w.Body.String(), "BAD_REQUEST") {
		t.Errorf("body missing BAD_REQUEST code: %s", w.Body.String())
	}
}

// TestHandlerListAll_Error verifies ListAll's error path renders the right status.
func TestHandlerListAll_Error(t *testing.T) {
	r := newTestRouter(&fakeService{err: apperr.ErrInternal.WithMessage("boom")})

	req := httptest.NewRequest(http.MethodGet, "/applications?page=1&limit=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

func TestHandlerUpdateStage_BadJSON(t *testing.T) {
	called := false
	fake := &fakeServiceWithFlag{flag: &called}
	r := chi.NewRouter()
	rt := svc.NewRestRouter(svc.NewRestService(fake), newTestZoneConfig())
	rt.Register(r)

	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/stage", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
	if called {
		t.Error("next service must not be called on malformed body")
	}
}

func TestHandlerUpdateStage_Error(t *testing.T) {
	r := newTestRouter(&fakeService{err: apperr.ErrNotFound.WithMessage("missing")})

	body, _ := json.Marshal(dto.UpdateStageInput{Stage: appcommon.StageApplied})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/stage", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

// TestHandlerUpdateStage_BodyEchoesAppIDStage verifies the success response
// body returns the appID + stage so the dashboard UI can keep its row in sync.
func TestHandlerUpdateStage_BodyEchoesAppIDStage(t *testing.T) {
	r := newTestRouter(&fakeService{})

	body, _ := json.Marshal(dto.UpdateStageInput{Stage: appcommon.StageOffer})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/77/stage", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"77"`) || !strings.Contains(w.Body.String(), appcommon.StageOffer) {
		t.Errorf("expected body to echo appID and stage, got %s", w.Body.String())
	}
}

func TestHandlerUpdateStageByID_BadJSON(t *testing.T) {
	called := false
	fake := &fakeServiceWithFlag{flag: &called}
	r := chi.NewRouter()
	rt := svc.NewRestRouter(svc.NewRestService(fake), newTestZoneConfig())
	rt.Register(r)

	req := httptest.NewRequest(http.MethodPatch, "/applications/1/stage", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
	if called {
		t.Error("next service must not be called on malformed body")
	}
}

func TestHandlerUpdateStageByID_Error(t *testing.T) {
	r := newTestRouter(&fakeService{err: apperr.ErrBadRequest.WithMessage("bad app id")})

	body, _ := json.Marshal(dto.UpdateStageInput{Stage: appcommon.StageApplied})
	req := httptest.NewRequest(http.MethodPatch, "/applications/abc/stage", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandlerUpdateNote_BadJSON(t *testing.T) {
	called := false
	fake := &fakeServiceWithFlag{flag: &called}
	r := chi.NewRouter()
	rt := svc.NewRestRouter(svc.NewRestService(fake), newTestZoneConfig())
	rt.Register(r)

	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/note", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
	if called {
		t.Error("next service must not be called on malformed body")
	}
}

func TestHandlerUpdateNote_Error(t *testing.T) {
	r := newTestRouter(&fakeService{err: apperr.ErrInternal.WithMessage("boom")})

	body, _ := json.Marshal(dto.UpdateNoteInput{Note: "x"})
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/note", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}

// TestHandlerListAll_PagedBodyShape — the response body must use the
// PagedResponse envelope: data, total, page, limit fields all present.
func TestHandlerListAll_PagedBodyShape(t *testing.T) {
	fake := &fakeService{appsWithJob: []svc.ApplicationWithJob{{JobTitle: "BE"}}, total: 42}
	r := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/applications?page=2&limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, key := range []string{`"data"`, `"total"`, `"page"`, `"limit"`, `"total":42`, `"page":2`, `"limit":10`} {
		if !strings.Contains(body, key) {
			t.Errorf("body missing %q: %s", key, body)
		}
	}
}

// TestHandlerCreate_Returns201 — Create must return 201 Created, not 200.
func TestHandlerCreate_Returns201(t *testing.T) {
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

// fakeServiceWithFlag observes whether the chain was invoked at all.
type fakeServiceWithFlag struct {
	fakeService
	flag *bool
}

func (f *fakeServiceWithFlag) ListByJob(ctx context.Context, jobID string) ([]appmodel.JobApplication, error) {
	*f.flag = true
	return f.fakeService.ListByJob(ctx, jobID)
}
func (f *fakeServiceWithFlag) ListAll(ctx context.Context, page, limit int) ([]svc.ApplicationWithJob, int, error) {
	*f.flag = true
	return f.fakeService.ListAll(ctx, page, limit)
}
func (f *fakeServiceWithFlag) ListActive(ctx context.Context) ([]svc.ApplicationWithJob, error) {
	*f.flag = true
	return f.fakeService.ListActive(ctx)
}
func (f *fakeServiceWithFlag) Create(ctx context.Context, jobID string, input dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	*f.flag = true
	return f.fakeService.Create(ctx, jobID, input)
}
func (f *fakeServiceWithFlag) UpdateStage(ctx context.Context, jobID, appID string, input dto.UpdateStageInput) error {
	*f.flag = true
	return f.fakeService.UpdateStage(ctx, jobID, appID, input)
}
func (f *fakeServiceWithFlag) UpdateStageByID(ctx context.Context, appID string, input dto.UpdateStageInput) error {
	*f.flag = true
	return f.fakeService.UpdateStageByID(ctx, appID, input)
}
func (f *fakeServiceWithFlag) UpdateNote(ctx context.Context, jobID, appID string, input dto.UpdateNoteInput) error {
	*f.flag = true
	return f.fakeService.UpdateNote(ctx, jobID, appID, input)
}

// Ensure test double implements interface.
var _ svc.ApplicationService = (*fakeService)(nil)
var _ svc.ApplicationService = (*fakeServiceWithFlag)(nil)
