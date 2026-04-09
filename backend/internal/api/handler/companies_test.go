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

func newCompanyHandler(cs *fakeCompanyStore, pub *fakePublisher) *CompanyHandler {
	return NewCompanyHandler(cs, &fakeScanLogReader{}, pub, 1440)
}

func testCompany(id string, lastScanAt *time.Time) model.Company {
	return model.Company{ID: id, Name: "Acme", URL: "https://example.com", AdapterType: model.AdapterGreenhouse, Active: true, LastScanAt: lastScanAt}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestCompanyList(t *testing.T) {
	cs := newFakeCompanyStore(testCompany("c1", nil), testCompany("c2", nil))
	h := newCompanyHandler(cs, &fakePublisher{})

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
	h := newCompanyHandler(cs, &fakePublisher{})

	body := strings.NewReader(`{"id":"stripe","name":"Stripe","url":"https://stripe.com","adapter_type":"greenhouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/companies", body)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201 got %d: %s", w.Code, w.Body.String())
	}
}

func TestCompanyCreate_MissingFields(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakePublisher{})

	body := strings.NewReader(`{"id":"stripe","adapter_type":"greenhouse"}`)
	req := httptest.NewRequest(http.MethodPost, "/companies", body)
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestCompanyCreate_InvalidAdapter(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(), &fakePublisher{})

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
	h := newCompanyHandler(cs, &fakePublisher{})

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
	h := newCompanyHandler(newFakeCompanyStore(), &fakePublisher{})

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
	cs := newFakeCompanyStore(testCompany("c1", nil)) // no prior scan
	pub := &fakePublisher{}
	h := newCompanyHandler(cs, pub)

	r := chi.NewRouter()
	r.Post("/companies/{id}/scan", h.TriggerScan)

	req := httptest.NewRequest(http.MethodPost, "/companies/c1/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202 got %d: %s", w.Code, w.Body.String())
	}
	if len(pub.payloads) != 1 {
		t.Errorf("expected 1 kafka publish, got %d", len(pub.payloads))
	}
}

func TestCompanyTriggerScan_Cooldown(t *testing.T) {
	recent := time.Now().Add(-10 * time.Minute) // 10m ago, interval is 1440m
	cs := newFakeCompanyStore(testCompany("c1", &recent))
	h := newCompanyHandler(cs, &fakePublisher{})

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
	h := newCompanyHandler(newFakeCompanyStore(), &fakePublisher{})

	r := chi.NewRouter()
	r.Post("/companies/{id}/scan", h.TriggerScan)

	req := httptest.NewRequest(http.MethodPost, "/companies/missing/scan", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w.Code)
	}
}
