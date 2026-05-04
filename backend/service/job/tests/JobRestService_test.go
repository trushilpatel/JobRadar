package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// fakeRestSvc is a JobService test double used to drive the REST layer.
// Captures inputs so handler-level wiring (filter parsing, body decoding)
// can be asserted without spinning up the impl chain.
type fakeRestSvc struct {
	listResult jobdto.PageResult
	listErr    error
	lastFilter jobdto.JobFilter
	lastCursor string

	getResult *jobdto.JobView
	getErr    error

	bulkErr    error
	bulkIDs    []string
	bulkReason string
}

func (f *fakeRestSvc) List(_ context.Context, filter jobdto.JobFilter, cursor string, _ int) (jobdto.PageResult, error) {
	f.lastFilter = filter
	f.lastCursor = cursor
	return f.listResult, f.listErr
}
func (f *fakeRestSvc) Get(_ context.Context, _ string) (*jobdto.JobView, error) {
	return f.getResult, f.getErr
}
func (f *fakeRestSvc) SetSaved(_ context.Context, _ string, _ bool) error { return nil }
func (f *fakeRestSvc) SetStatus(_ context.Context, _, _ string) error     { return nil }
func (f *fakeRestSvc) SetNotes(_ context.Context, _, _ string) error      { return nil }
func (f *fakeRestSvc) Dismiss(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (f *fakeRestSvc) BulkDismiss(_ context.Context, ids []string, reason, _ string) error {
	f.bulkIDs = ids
	f.bulkReason = reason
	return f.bulkErr
}
func (f *fakeRestSvc) GetCounts(_ context.Context) (jobdto.Counts, error) {
	return jobdto.Counts{}, nil
}

var _ jobsvc.JobService = (*fakeRestSvc)(nil)

func newRouter(svc jobsvc.JobService) *chi.Mux {
	rest := jobsvc.NewRestService(svc)
	router := jobsvc.NewRestRouter(rest)
	r := chi.NewRouter()
	router.Register(r)
	return r
}

func TestRest_List_OK_HasNextCursor(t *testing.T) {
	r := newRouter(&fakeRestSvc{listResult: jobdto.PageResult{Items: []jobdto.JobView{}, NextCursor: "abc"}})
	req := httptest.NewRequest(http.MethodGet, "/jobs?cursor=xyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"next_cursor":"abc"`) {
		t.Errorf("response missing next_cursor: %s", w.Body.String())
	}
}

func TestRest_List_ForwardsCursor(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)
	req := httptest.NewRequest(http.MethodGet, "/jobs?cursor=opaque-cursor", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if fake.lastCursor != "opaque-cursor" {
		t.Errorf("cursor = %q, want %q", fake.lastCursor, "opaque-cursor")
	}
}

func TestRest_List_ServiceErrorMapsTo500(t *testing.T) {
	r := newRouter(&fakeRestSvc{listErr: apperr.ErrInternal.WithMessage("db down")})
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestRest_BulkDismiss_DefaultReason(t *testing.T) {
	fake := &fakeRestSvc{}
	r := newRouter(fake)
	body, _ := json.Marshal(map[string][]string{"ids": {"j1", "j2"}})
	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if fake.bulkReason != jobcommon.DismissalReasonNotRelevant {
		t.Errorf("default reason = %q, want %q", fake.bulkReason, jobcommon.DismissalReasonNotRelevant)
	}
	if len(fake.bulkIDs) != 2 {
		t.Errorf("ids = %v, want 2 entries", fake.bulkIDs)
	}
}

func TestRest_BulkDismiss_BadJSON(t *testing.T) {
	r := newRouter(&fakeRestSvc{})
	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRest_Get_NotFound(t *testing.T) {
	r := newRouter(&fakeRestSvc{getErr: apperr.ErrNotFound.WithMessage("missing")})
	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
