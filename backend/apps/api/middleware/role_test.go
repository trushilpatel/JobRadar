package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trushilpatel/jobradar/common/identity"
)

func TestRequireRole_Allowed(t *testing.T) {
	mw := RequireRole("admin")
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/x", nil)
	req = req.WithContext(identity.WithTestIdentity(req.Context(), "u1", "user", "admin"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("next handler must be called when role is granted")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200", rr.Code)
	}
}

func TestRequireRole_MissingRole_403(t *testing.T) {
	mw := RequireRole("admin")
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/x", nil)
	req = req.WithContext(identity.WithTestIdentity(req.Context(), "u1", "user"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if called {
		t.Fatal("next handler must NOT be called when role is missing")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403", rr.Code)
	}
}

func TestRequireRole_NoIdentity_403(t *testing.T) {
	mw := RequireRole("admin")
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// No identity attached at all — must surface as 403, not panic.
	req := httptest.NewRequest(http.MethodGet, "/admin/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if called {
		t.Fatal("next handler must NOT be called when identity is absent")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403", rr.Code)
	}
}

func TestRequireRole_EmptyRole_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("RequireRole(\"\") must panic")
		}
	}()
	RequireRole("")
}
