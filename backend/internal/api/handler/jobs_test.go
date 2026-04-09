package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/model"
)

func newJobHandler(js *fakeJobStore) *JobHandler {
	return NewJobHandler(js, &fakeLedgerWriter{}, &fakeDiscardStore{}, &fakeSnapshotWriter{})
}

func jobWithID(id, status string) *model.Job {
	postID := "greenhouse-" + id
	return &model.Job{ID: id, JobPostID: postID, Title: "SWE", Status: status, CompanyID: "c1"}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestJobList_DefaultStatus(t *testing.T) {
	js := newFakeJobStore(jobWithID("1", model.JobStatusNew), jobWithID("2", model.JobStatusSaved))
	h := newJobHandler(js)

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	var jobs []model.Job
	json.NewDecoder(w.Body).Decode(&jobs)
	if len(jobs) != 1 || jobs[0].ID != "1" {
		t.Errorf("expected only new job, got %+v", jobs)
	}
}

func TestJobList_ValidStatus(t *testing.T) {
	js := newFakeJobStore(jobWithID("1", model.JobStatusNew), jobWithID("2", model.JobStatusSaved))
	h := newJobHandler(js)

	req := httptest.NewRequest(http.MethodGet, "/jobs?status=saved", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	var jobs []model.Job
	json.NewDecoder(w.Body).Decode(&jobs)
	if len(jobs) != 1 || jobs[0].ID != "2" {
		t.Errorf("expected saved job, got %+v", jobs)
	}
}

func TestJobList_InvalidStatus(t *testing.T) {
	h := newJobHandler(newFakeJobStore())

	req := httptest.NewRequest(http.MethodGet, "/jobs?status=unknown", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestJobGet_Found(t *testing.T) {
	js := newFakeJobStore(jobWithID("job-1", model.JobStatusNew))
	h := newJobHandler(js)

	r := chi.NewRouter()
	r.Get("/jobs/{id}", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

func TestJobGet_NotFound(t *testing.T) {
	h := newJobHandler(newFakeJobStore())

	r := chi.NewRouter()
	r.Get("/jobs/{id}", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w.Code)
	}
}

// ── SetSaved ──────────────────────────────────────────────────────────────────

func TestJobSetSaved_True(t *testing.T) {
	js := newFakeJobStore(jobWithID("j1", model.JobStatusNew))
	snap := &fakeSnapshotWriter{}
	h := NewJobHandler(js, &fakeLedgerWriter{}, &fakeDiscardStore{}, snap)

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/save", h.SetSaved)

	body := strings.NewReader(`{"saved":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/save", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if snap.lastAction != "saved" {
		t.Errorf("expected snapshot action 'saved', got %q", snap.lastAction)
	}
}

func TestJobSetSaved_False_NoSnapshotUpdate(t *testing.T) {
	js := newFakeJobStore(jobWithID("j1", model.JobStatusSaved))
	snap := &fakeSnapshotWriter{}
	h := NewJobHandler(js, &fakeLedgerWriter{}, &fakeDiscardStore{}, snap)

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/save", h.SetSaved)

	body := strings.NewReader(`{"saved":false}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/save", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if snap.lastAction != "" {
		t.Errorf("expected no snapshot update, got %q", snap.lastAction)
	}
}

// ── SetStatus ─────────────────────────────────────────────────────────────────

func TestJobSetStatus_Valid(t *testing.T) {
	js := newFakeJobStore(jobWithID("j1", model.JobStatusNew))
	snap := &fakeSnapshotWriter{}
	h := NewJobHandler(js, &fakeLedgerWriter{}, &fakeDiscardStore{}, snap)

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/status", h.SetStatus)

	body := strings.NewReader(`{"status":"applied"}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/status", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if snap.lastAction != "applied" {
		t.Errorf("expected snapshot action 'applied', got %q", snap.lastAction)
	}
}

func TestJobSetStatus_Invalid(t *testing.T) {
	h := newJobHandler(newFakeJobStore())

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/status", h.SetStatus)

	body := strings.NewReader(`{"status":"dismissed"}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/status", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

// ── SetNotes ──────────────────────────────────────────────────────────────────

func TestJobSetNotes(t *testing.T) {
	js := newFakeJobStore(jobWithID("j1", model.JobStatusNew))
	h := newJobHandler(js)

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/notes", h.SetNotes)

	body := strings.NewReader(`{"notes":"follow up next week"}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/notes", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if js.notes != "follow up next week" {
		t.Errorf("notes not updated, got %q", js.notes)
	}
}

// ── Dismiss ───────────────────────────────────────────────────────────────────

func TestJobDismiss_Valid(t *testing.T) {
	js := newFakeJobStore(jobWithID("j1", model.JobStatusNew))
	discard := &fakeDiscardStore{}
	ledger := &fakeLedgerWriter{}
	snap := &fakeSnapshotWriter{}
	h := NewJobHandler(js, ledger, discard, snap)

	r := chi.NewRouter()
	r.Delete("/jobs/{id}", h.Dismiss)

	body := strings.NewReader(`{"reason":"user_dismissed","note":"not interested"}`)
	req := httptest.NewRequest(http.MethodDelete, "/jobs/j1", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	if !discard.called {
		t.Error("expected discard record to be inserted")
	}
	if ledger.lastReason != model.SeenReasonUserDismissed {
		t.Errorf("expected seen_ledger reason=%q, got %q", model.SeenReasonUserDismissed, ledger.lastReason)
	}
	if snap.lastAction != "dismissed" {
		t.Errorf("expected snapshot action 'dismissed', got %q", snap.lastAction)
	}
}

func TestJobDismiss_InvalidReason(t *testing.T) {
	h := newJobHandler(newFakeJobStore(jobWithID("j1", model.JobStatusNew)))

	r := chi.NewRouter()
	r.Delete("/jobs/{id}", h.Dismiss)

	body := strings.NewReader(`{"reason":"bad_reason"}`)
	req := httptest.NewRequest(http.MethodDelete, "/jobs/j1", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestJobDismiss_NotFound(t *testing.T) {
	h := newJobHandler(newFakeJobStore())

	r := chi.NewRouter()
	r.Delete("/jobs/{id}", h.Dismiss)

	body := strings.NewReader(`{"reason":"user_dismissed"}`)
	req := httptest.NewRequest(http.MethodDelete, "/jobs/missing", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w.Code)
	}
}
