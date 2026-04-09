package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInsightsTechStack_DefaultLimit(t *testing.T) {
	ins := &fakeInsightsReader{}
	h := NewInsightsHandler(ins)

	req := httptest.NewRequest(http.MethodGet, "/insights/tech-stack", nil)
	w := httptest.NewRecorder()
	h.TechStack(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if ins.techLimit != 15 {
		t.Errorf("expected default limit=15, got %d", ins.techLimit)
	}
}

func TestInsightsTechStack_CustomLimit(t *testing.T) {
	ins := &fakeInsightsReader{}
	h := NewInsightsHandler(ins)

	req := httptest.NewRequest(http.MethodGet, "/insights/tech-stack?limit=5", nil)
	w := httptest.NewRecorder()
	h.TechStack(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if ins.techLimit != 5 {
		t.Errorf("expected limit=5, got %d", ins.techLimit)
	}
}

func TestInsightsTechStack_InvalidLimit_UsesDefault(t *testing.T) {
	ins := &fakeInsightsReader{}
	h := NewInsightsHandler(ins)

	req := httptest.NewRequest(http.MethodGet, "/insights/tech-stack?limit=abc", nil)
	w := httptest.NewRecorder()
	h.TechStack(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if ins.techLimit != 15 {
		t.Errorf("invalid limit should fall back to 15, got %d", ins.techLimit)
	}
}

func TestInsightsVelocity_DefaultLimit(t *testing.T) {
	ins := &fakeInsightsReader{}
	h := NewInsightsHandler(ins)

	req := httptest.NewRequest(http.MethodGet, "/insights/velocity", nil)
	w := httptest.NewRecorder()
	h.Velocity(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if ins.velocityLimit != 20 {
		t.Errorf("expected default limit=20, got %d", ins.velocityLimit)
	}
}

func TestInsightsVelocity_CustomLimit(t *testing.T) {
	ins := &fakeInsightsReader{}
	h := NewInsightsHandler(ins)

	req := httptest.NewRequest(http.MethodGet, "/insights/velocity?limit=10", nil)
	w := httptest.NewRecorder()
	h.Velocity(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if ins.velocityLimit != 10 {
		t.Errorf("expected limit=10, got %d", ins.velocityLimit)
	}
}

func TestInsightsFunnel(t *testing.T) {
	h := NewInsightsHandler(&fakeInsightsReader{})

	req := httptest.NewRequest(http.MethodGet, "/insights/funnel", nil)
	w := httptest.NewRecorder()
	h.Funnel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

func TestInsightsSponsorship_CustomDays(t *testing.T) {
	h := NewInsightsHandler(&fakeInsightsReader{})

	req := httptest.NewRequest(http.MethodGet, "/insights/sponsorship?days=7", nil)
	w := httptest.NewRecorder()
	h.Sponsorship(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}
