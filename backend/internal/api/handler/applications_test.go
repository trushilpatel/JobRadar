package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/model"
)

func newAppHandler(apps *fakeApplicationStore) *ApplicationHandler {
	js := newFakeJobStore(jobWithID("j1", model.JobStatusSaved))
	return NewApplicationHandler(apps, js)
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestAppCreate_ValidStage(t *testing.T) {
	apps := &fakeApplicationStore{}
	h := newAppHandler(apps)

	r := chi.NewRouter()
	r.Post("/jobs/{id}/applications", h.Create)

	body := strings.NewReader(`{"stage":"phone_screen","note":"great call"}`)
	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/applications", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201 got %d: %s", w.Code, w.Body.String())
	}
	if apps.lastStage != model.StagePhoneScreen {
		t.Errorf("expected stage=%q, got %q", model.StagePhoneScreen, apps.lastStage)
	}
}

func TestAppCreate_AppliedStage_AdvancesJobStatus(t *testing.T) {
	apps := &fakeApplicationStore{}
	js := newFakeJobStore(jobWithID("j1", model.JobStatusSaved))
	h := NewApplicationHandler(apps, js)

	r := chi.NewRouter()
	r.Post("/jobs/{id}/applications", h.Create)

	body := strings.NewReader(`{"stage":"applied"}`)
	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/applications", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201 got %d", w.Code)
	}
	if js.status != model.JobStatusApplied {
		t.Errorf("expected job status to be updated to 'applied', got %q", js.status)
	}
}

func TestAppCreate_InvalidStage(t *testing.T) {
	h := newAppHandler(&fakeApplicationStore{})

	r := chi.NewRouter()
	r.Post("/jobs/{id}/applications", h.Create)

	body := strings.NewReader(`{"stage":"unknown"}`)
	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/applications", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

// ── UpdateStage ───────────────────────────────────────────────────────────────

func TestAppUpdateStage_Valid(t *testing.T) {
	apps := &fakeApplicationStore{}
	h := newAppHandler(apps)

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/applications/{app_id}/stage", h.UpdateStage)

	body := strings.NewReader(`{"stage":"technical"}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/42/stage", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if apps.lastStage != model.StageTechnical {
		t.Errorf("expected stage=%q, got %q", model.StageTechnical, apps.lastStage)
	}
}

func TestAppUpdateStage_InvalidAppID(t *testing.T) {
	h := newAppHandler(&fakeApplicationStore{})

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/applications/{app_id}/stage", h.UpdateStage)

	body := strings.NewReader(`{"stage":"technical"}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/not-a-number/stage", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestAppUpdateStage_InvalidStage(t *testing.T) {
	h := newAppHandler(&fakeApplicationStore{})

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/applications/{app_id}/stage", h.UpdateStage)

	body := strings.NewReader(`{"stage":"bogus"}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/1/stage", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

// ── UpdateNote ────────────────────────────────────────────────────────────────

func TestAppUpdateNote(t *testing.T) {
	apps := &fakeApplicationStore{}
	h := newAppHandler(apps)

	r := chi.NewRouter()
	r.Patch("/jobs/{id}/applications/{app_id}/note", h.UpdateNote)

	body := strings.NewReader(`{"note":"great culture fit"}`)
	req := httptest.NewRequest(http.MethodPatch, "/jobs/j1/applications/7/note", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if apps.lastNote != "great culture fit" {
		t.Errorf("expected note to be saved, got %q", apps.lastNote)
	}
}
