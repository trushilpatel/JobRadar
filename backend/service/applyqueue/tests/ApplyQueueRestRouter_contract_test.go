package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	applyqueuedto "github.com/trushilpatel/jobradar/service/applyqueue/dto"
	applyqueuesvc "github.com/trushilpatel/jobradar/service/applyqueue/services"
)

// TestApplyQueueRest_ContractAppearsInOpenAPI is the Slice 4 per-slice
// quality gate's contract test.
func TestApplyQueueRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := applyqueuesvc.NewRestService(&fakeRestService{})
	router := applyqueuesvc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	listItem, ok := zoneCfg.Paths["/apply-queue"]
	if !ok || listItem.Get == nil {
		t.Fatalf("expected GET /apply-queue in the generated spec, got: %v", zoneCfg.Paths)
	}
	if listItem.Get.OperationID != "listApplyQueue" {
		t.Errorf("OperationID = %q, want listApplyQueue", listItem.Get.OperationID)
	}

	resumeItem, ok := zoneCfg.Paths["/apply-queue/{jobId}/resume"]
	if !ok || resumeItem.Put == nil {
		t.Fatalf("expected PUT /apply-queue/{jobId}/resume in the generated spec, got: %v", zoneCfg.Paths)
	}
	if resumeItem.Put.OperationID != "setApplyQueueResume" {
		t.Errorf("OperationID = %q, want setApplyQueueResume", resumeItem.Put.OperationID)
	}
}

// TestApplyQueueRest_List_LimitClampedToMax is the deliberate, plan-approved
// behavior tightening from collapsing this slice's independent inline limit
// parsing onto the shared apps/api/request.ParseLimit: an out-of-range
// limit is now clamped to 100 instead of forwarded unbounded (the pre-Huma
// handler had no upper clamp at all — no existing test relied on that).
func TestApplyQueueRest_List_LimitClampedToMax(t *testing.T) {
	fake := &fakeRestService{}
	mux := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/apply-queue?limit=999", nil)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.lastLimit != 100 {
		t.Errorf("limit = %d, want clamped to 100", fake.lastLimit)
	}
}

// TestApplyQueueRest_List_GoldenEnvelope locks the exact byte shape of
// GET /api/apply-queue, satisfying the envelope-parity gate item.
func TestApplyQueueRest_List_GoldenEnvelope(t *testing.T) {
	fake := &fakeRestService{listResult: applyqueuedto.QueueListResult{Items: []applyqueuedto.QueueItem{}, NextCursor: "n2"}}
	mux := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/apply-queue", nil)
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	want := `{"data":{"items":[],"next_cursor":"n2"}}` + "\n"
	if rec.Body.String() != want {
		t.Fatalf("golden envelope mismatch\ngot:  %q\nwant: %q", rec.Body.String(), want)
	}
}

// TestApplyQueueRest_SetResume_Clear_TrulyEmptyBody guards the
// Body func(huma.Context) design decision: the 204 response must have a
// genuinely empty body, not the literal 4 bytes "null" a naive
// *dto.AttachmentView Body field would have produced for a nil pointer.
func TestApplyQueueRest_SetResume_Clear_TrulyEmptyBody(t *testing.T) {
	fake := &fakeRestService{setResumeView: nil}
	mux := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodPut, "/apply-queue/j1/resume", strings.NewReader(`{"resume_id":null}`))
	req = withUser(req, "u1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected a truly empty body, got %d bytes: %q", rec.Body.Len(), rec.Body.String())
	}
}
