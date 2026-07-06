package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	applyqueuedto "github.com/trushilpatel/jobradar/service/applyqueue/dto"
	applyqueuesvc "github.com/trushilpatel/jobradar/service/applyqueue/services"
)

// fakeRestService is a configurable Service double for RestService HTTP tests.
type fakeRestService struct {
	listResult applyqueuedto.QueueListResult
	listErr    error
	lastUserID string
	lastFilter string
	lastCursor string
	lastLimit  int

	setResumeView *applyqueuedto.AttachmentView
	setResumeErr  error
	lastJobID     string
	lastResumeID  *string
}

func (f *fakeRestService) List(_ context.Context, userID, filter, cursor string, limit int) (applyqueuedto.QueueListResult, error) {
	f.lastUserID = userID
	f.lastFilter = filter
	f.lastCursor = cursor
	f.lastLimit = limit
	return f.listResult, f.listErr
}

func (f *fakeRestService) SetResume(_ context.Context, userID, jobID string, resumeID *string) (*applyqueuedto.AttachmentView, error) {
	f.lastUserID = userID
	f.lastJobID = jobID
	f.lastResumeID = resumeID
	return f.setResumeView, f.setResumeErr
}

var _ applyqueuesvc.Service = (*fakeRestService)(nil)

func withUser(r *http.Request, userID string) *http.Request {
	return r.WithContext(identity.WithTestIdentity(r.Context(), userID))
}

func newTestRouter(fake *fakeRestService) *chi.Mux {
	h := applyqueuesvc.NewRestService(fake)
	router := applyqueuesvc.NewRestRouter(h)
	mux := chi.NewRouter()
	router.Register(mux)
	return mux
}

func TestApplyQueueRest_List_DefaultsFilterAndLimit(t *testing.T) {
	fake := &fakeRestService{listResult: applyqueuedto.QueueListResult{NextCursor: "n1"}}
	mux := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/apply-queue", nil)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.lastFilter != "all" {
		t.Errorf("filter = %q, want all", fake.lastFilter)
	}
	if fake.lastLimit != 20 {
		t.Errorf("limit = %d, want 20", fake.lastLimit)
	}
	if fake.lastUserID != "u1" {
		t.Errorf("userID = %q, want u1", fake.lastUserID)
	}
}

func TestApplyQueueRest_List_ParsesFilterCursorLimit(t *testing.T) {
	fake := &fakeRestService{}
	mux := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/apply-queue?filter=saved&cursor=abc&limit=5", nil)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fake.lastFilter != "saved" || fake.lastCursor != "abc" || fake.lastLimit != 5 {
		t.Errorf("parsed wrong: filter=%q cursor=%q limit=%d", fake.lastFilter, fake.lastCursor, fake.lastLimit)
	}
}

func TestApplyQueueRest_List_InvalidLimit_FallsBackToDefault(t *testing.T) {
	fake := &fakeRestService{}
	mux := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/apply-queue?limit=not-a-number", nil)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fake.lastLimit != 20 {
		t.Errorf("limit = %d, want default 20", fake.lastLimit)
	}
}

func TestApplyQueueRest_List_ErrorFromService_HandledViaHandleError(t *testing.T) {
	fake := &fakeRestService{listErr: domainerrs.ErrInternal.WithMessage("boom")}
	mux := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/apply-queue", nil)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestApplyQueueRest_SetResume_Attach_Returns200WithView(t *testing.T) {
	resumeID := "r1"
	fake := &fakeRestService{setResumeView: &applyqueuedto.AttachmentView{JobID: "j1", ResumeID: &resumeID}}
	mux := newTestRouter(fake)

	body := bytes.NewBufferString(`{"resume_id":"r1"}`)
	req := httptest.NewRequest(http.MethodPut, "/apply-queue/j1/resume", body)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.lastJobID != "j1" || fake.lastResumeID == nil || *fake.lastResumeID != "r1" {
		t.Errorf("forwarded wrong: jobID=%q resumeID=%v", fake.lastJobID, fake.lastResumeID)
	}
}

func TestApplyQueueRest_SetResume_Clear_Returns204(t *testing.T) {
	fake := &fakeRestService{setResumeView: nil}
	mux := newTestRouter(fake)

	body := bytes.NewBufferString(`{"resume_id":null}`)
	req := httptest.NewRequest(http.MethodPut, "/apply-queue/j1/resume", body)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.lastResumeID != nil {
		t.Errorf("expected nil resumeID forwarded, got %v", fake.lastResumeID)
	}
}

func TestApplyQueueRest_SetResume_InvalidBody_Returns400(t *testing.T) {
	fake := &fakeRestService{}
	mux := newTestRouter(fake)

	body := bytes.NewBufferString(`not-json`)
	req := httptest.NewRequest(http.MethodPut, "/apply-queue/j1/resume", body)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestApplyQueueRest_SetResume_ErrorFromService_HandledViaHandleError(t *testing.T) {
	fake := &fakeRestService{setResumeErr: domainerrs.ErrNotFound.WithMessage("resume not found")}
	mux := newTestRouter(fake)

	body := bytes.NewBufferString(`{"resume_id":"r1"}`)
	req := httptest.NewRequest(http.MethodPut, "/apply-queue/j1/resume", body)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestNewRestService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil next")
		}
	}()
	_ = applyqueuesvc.NewRestService(nil)
}
