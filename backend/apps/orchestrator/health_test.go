package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
)

// stubNATS is a minimal natsConnStatus stub for tests that don't need a real
// NATS server. status controls what Status() returns.
type stubNATS struct {
	status nats.Status
}

func (s *stubNATS) Status() nats.Status { return s.status }

func TestHealthzHandler_ReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthzHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("want body %q, got %q", "ok", got)
	}
}

// TestReadyzHandler_NilConn verifies 503 is returned when nc is nil.
func TestReadyzHandler_NilConn(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	readyzHandler(nil)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

// TestReadyzHandler_Disconnected verifies 503 when the conn is not CONNECTED.
func TestReadyzHandler_Disconnected(t *testing.T) {
	for _, st := range []nats.Status{nats.CLOSED, nats.DISCONNECTED, nats.RECONNECTING} {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		readyzHandler(&stubNATS{status: st})(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status %v: want 503, got %d", st, rec.Code)
		}
	}
}

// TestReadyzHandler_Connected verifies 200 when NATS reports CONNECTED.
func TestReadyzHandler_Connected(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	readyzHandler(&stubNATS{status: nats.CONNECTED})(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("want body %q, got %q", "ok", got)
	}
}

// TestHealthRoutes_RouterRegistration is the regression test for the /healthz
// route-missing incident. It verifies registerHealthRoutes wires both routes
// onto the router. If either route registration is removed from health.go,
// this test fails.
func TestHealthRoutes_RouterRegistration(t *testing.T) {
	r := chi.NewRouter()
	registerHealthRoutes(r, nil) // nil nc → /readyz returns 503, but route IS registered

	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Errorf("route %s not registered (got 404)", path)
		}
	}
}

// TestHealthRoutes_RouterRegistration_NonNilConn exercises the nc != nil
// branch of registerHealthRoutes — a zero-value *nats.Conn is never
// connected (Status() reads a zero-value field, no network I/O), so /readyz
// still 503s, but through the "wrap nc in natsConnStatus" path rather than
// the nil-interface short-circuit TestHealthRoutes_RouterRegistration covers.
func TestHealthRoutes_RouterRegistration_NonNilConn(t *testing.T) {
	r := chi.NewRouter()
	registerHealthRoutes(r, &nats.Conn{})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 for a never-connected *nats.Conn", rec.Code)
	}
}
