package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/model"
)

func newCompanyHandler(cs *fakeCompanyStore, q *fakeScanQueueStore) *CompanyHandler {
	return NewCompanyHandler(cs, &fakeScanLogReader{}, q, 1440)
}

func testCompany(id string, lastScanAt *time.Time) model.Company {
	return model.Company{ID: id, Name: "Acme", URL: "https://example.com", AdapterType: model.AdapterGreenhouse, Active: true, LastScanAt: lastScanAt}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestCompanyList(t *testing.T) {
	cs := newFakeCompanyStore(testCompany("c1", nil), testCompany("c2", nil))
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCompanyCreate_Valid(t *testing.T) {
	cs := newFakeCompanyStore()
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	body := strings.NewReader(`{"id":"stripe","name":"Stripe","url":"https://stripe.com","adapter_type":"greenhouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/companies", body)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201 got %d: %s", w.Code, w.Body.String())
	}
}

func TestCompanyCreate_MissingFields(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	body := strings.NewReader(`{"id":"stripe","adapter_type":"greenhouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/companies", body)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestCompanyCreate_InvalidAdapter(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	body := strings.NewReader(`{"id":"stripe","name":"Stripe","url":"https://stripe.com","adapter_type":"unknown"}`)
	req := httptest.NewRequest(http.MethodPost, "/companies", body)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestCompanyDelete_Found(t *testing.T) {
	cs := newFakeCompanyStore(testCompany("c1", nil))
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	r := chi.NewRouter()
	r.Delete("/companies/{id}", h.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/companies/c1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

func TestCompanyDelete_NotFound(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	r := chi.NewRouter()
	r.Delete("/companies/{id}", h.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/companies/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w.Code)
	}
}

// ── TriggerScan ───────────────────────────────────────────────────────────────

func TestCompanyTriggerScan_Success(t *testing.T) {
	cs := newFakeCompanyStore(testCompany("c1", nil))
	q := &fakeScanQueueStore{}
	h := newCompanyHandler(cs, q)

	r := chi.NewRouter()
	r.Post("/companies/{id}/scan", h.TriggerScan)

	req := httptest.NewRequest(http.MethodPost, "/companies/c1/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202 got %d: %s", w.Code, w.Body.String())
	}
	if len(q.items) != 1 {
		t.Fatalf("expected 1 queue item, got %d", len(q.items))
	}
	if q.items[0].CompanyID != "c1" {
		t.Errorf("expected company_id c1, got %s", q.items[0].CompanyID)
	}
	if q.items[0].Source != model.QueueSourceManual {
		t.Errorf("expected source=%q, got %q", model.QueueSourceManual, q.items[0].Source)
	}
}

func TestCompanyTriggerScan_Cooldown(t *testing.T) {
	recent := time.Now().Add(-10 * time.Minute)
	cs := newFakeCompanyStore(testCompany("c1", &recent))
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	r := chi.NewRouter()
	r.Post("/companies/{id}/scan", h.TriggerScan)

	req := httptest.NewRequest(http.MethodPost, "/companies/c1/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 got %d", w.Code)
	}
}

func TestCompanyTriggerScan_NotFound(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	r := chi.NewRouter()
	r.Post("/companies/{id}/scan", h.TriggerScan)

	req := httptest.NewRequest(http.MethodPost, "/companies/missing/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w.Code)
	}
}

// ── SetAllActive ──────────────────────────────────────────────────────────────

func TestCompanySetAllActive_Activate(t *testing.T) {
	inactive := testCompany("c1", nil)
	inactive.Active = false
	cs := newFakeCompanyStore(inactive, testCompany("c2", nil))
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	body := strings.NewReader(`{"active":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", body)
	w := httptest.NewRecorder()
	h.SetAllActive(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	for _, c := range cs.companies {
		if !c.Active {
			t.Errorf("company %s should be active", c.ID)
		}
	}
}

func TestCompanySetAllActive_Deactivate(t *testing.T) {
	cs := newFakeCompanyStore(testCompany("c1", nil), testCompany("c2", nil))
	h := newCompanyHandler(cs, &fakeScanQueueStore{})

	body := strings.NewReader(`{"active":false}`)
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", body)
	w := httptest.NewRecorder()
	h.SetAllActive(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	for _, c := range cs.companies {
		if c.Active {
			t.Errorf("company %s should be inactive", c.ID)
		}
	}
}

func TestCompanySetAllActive_BadRequest(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest(http.MethodPatch, "/companies/active", body)
	w := httptest.NewRecorder()
	h.SetAllActive(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

// ── ListQueue ─────────────────────────────────────────────────────────────────

func TestCompanyListQueue_Empty(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakeScanQueueStore{})

	req := httptest.NewRequest(http.MethodGet, "/companies/queue", nil)
	w := httptest.NewRecorder()
	h.ListQueue(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

func TestCompanyListQueue_WithItems(t *testing.T) {
	q := &fakeScanQueueStore{}
	_ = q.Enqueue("c1", model.QueueSourceScheduler)
	_ = q.Enqueue("c2", model.QueueSourceManual)
	h := newCompanyHandler(newFakeCompanyStore(), q)

	req := httptest.NewRequest(http.MethodGet, "/companies/queue", nil)
	w := httptest.NewRecorder()
	h.ListQueue(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
}
