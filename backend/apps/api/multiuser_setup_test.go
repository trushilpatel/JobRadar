package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
)

// TestMultiUser_APIHandlers_NilSafe verifies the getter tolerates a zero
// receiver — callers in main.go chain APIHandlers() onto app.Handlers() and
// must not crash when the multi-user subsystem is disabled.
func TestMultiUser_APIHandlers_NilSafe(t *testing.T) {
	var m *multiUser
	if got := m.APIHandlers(); got != nil {
		t.Fatalf("nil receiver: APIHandlers()=%v, want nil", got)
	}
}

// TestMultiUser_APIHandlers_NilRouter — receiver is non-nil but router slot is
// nil. Must still return nil (no panic), allowing the caller to append safely.
func TestMultiUser_APIHandlers_NilRouter(t *testing.T) {
	m := &multiUser{}
	if got := m.APIHandlers(); got != nil {
		t.Fatalf("nil router: APIHandlers()=%v, want nil", got)
	}
}

// TestMultiUser_APIHandlers_ReturnsRouter — happy path, returns the single
// /me group in a slice ready to mount.
func TestMultiUser_APIHandlers_ReturnsRouter(t *testing.T) {
	stub := &stubGroup{register: func(_ chi.Router) {}}
	m := &multiUser{router: stub}
	groups := m.APIHandlers()
	if len(groups) != 1 {
		t.Fatalf("APIHandlers()=%d groups, want 1", len(groups))
	}
}

func TestMultiUser_AdminHandlers_NilSafe(t *testing.T) {
	var m *multiUser
	if got := m.AdminHandlers(); got != nil {
		t.Fatalf("nil receiver: AdminHandlers()=%v, want nil", got)
	}
}

func TestMultiUser_AdminHandlers_NilAdmin(t *testing.T) {
	m := &multiUser{}
	if got := m.AdminHandlers(); got != nil {
		t.Fatalf("nil admin: AdminHandlers()=%v, want nil", got)
	}
}

func TestMultiUser_AdminHandlers_ReturnsAdmin(t *testing.T) {
	stub := &stubGroup{register: func(_ chi.Router) {}}
	m := &multiUser{admin: stub}
	groups := m.AdminHandlers()
	if len(groups) != 1 {
		t.Fatalf("AdminHandlers()=%d, want 1", len(groups))
	}
}

func TestMultiUser_IdentityMWAndRateLimitMW_PassThrough(t *testing.T) {
	var seen int32
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&seen, 1)
			next.ServeHTTP(w, r)
		})
	}
	m := &multiUser{identityMW: mw, rateLimitMW: mw}

	// IdentityMW
	idMW := m.IdentityMW()
	if idMW == nil {
		t.Fatal("IdentityMW returned nil")
	}
	// RateLimitMW
	rlMW := m.RateLimitMW()
	if rlMW == nil {
		t.Fatal("RateLimitMW returned nil")
	}

	// Drive a request through both — must increment the counter twice.
	h := idMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if atomic.LoadInt32(&seen) != 2 {
		t.Fatalf("middleware invocations=%d want 2", atomic.LoadInt32(&seen))
	}
}

// TestAdminRouteGroup_Register_AppliesAllMiddleware exercises the chained
// /admin subrouter — identity → role → rate → audit must all be invoked once
// before reaching the handler.
func TestAdminRouteGroup_Register_AppliesAllMiddleware(t *testing.T) {
	var identityHits, roleHits, rateHits, auditHits, handlerHits atomic.Int32
	tag := func(c *atomic.Int32) func(http.Handler) http.Handler {
		return markingMW("x", c)
	}

	innerGroup := &stubGroup{register: func(r chi.Router) {
		r.Get("/companies", func(w http.ResponseWriter, _ *http.Request) {
			handlerHits.Add(1)
			w.WriteHeader(http.StatusOK)
		})
	}}

	group := &adminRouteGroup{
		identityMW: tag(&identityHits),
		roleMW:     tag(&roleHits),
		rateMW:     tag(&rateHits),
		auditMW:    tag(&auditHits),
		handlers:   []handler.RouteGroup{innerGroup},
	}

	r := chi.NewRouter()
	group.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/companies", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if handlerHits.Load() != 1 {
		t.Fatalf("handler hits=%d want 1", handlerHits.Load())
	}
	if identityHits.Load() != 1 || roleHits.Load() != 1 || rateHits.Load() != 1 || auditHits.Load() != 1 {
		t.Fatalf("middleware hits: identity=%d role=%d rate=%d audit=%d (want all 1)",
			identityHits.Load(), roleHits.Load(), rateHits.Load(), auditHits.Load())
	}
}

// TestAdminRouteGroup_Register_NilMiddlewareSkipped verifies the conditional
// branches in Register — nil entries are silently dropped, so a group built
// with only a roleMW does not panic and still mounts its handlers.
func TestAdminRouteGroup_Register_NilMiddlewareSkipped(t *testing.T) {
	var roleHits, handlerHits atomic.Int32
	innerGroup := &stubGroup{register: func(r chi.Router) {
		r.Get("/x", func(w http.ResponseWriter, _ *http.Request) {
			handlerHits.Add(1)
			w.WriteHeader(http.StatusOK)
		})
	}}

	group := &adminRouteGroup{
		identityMW: nil,
		roleMW:     markingMW("role", &roleHits),
		rateMW:     nil,
		auditMW:    nil,
		handlers:   []handler.RouteGroup{innerGroup},
	}
	r := chi.NewRouter()
	group.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/x", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200 with only role middleware", rr.Code)
	}
	if roleHits.Load() != 1 || handlerHits.Load() != 1 {
		t.Fatalf("role=%d handler=%d want 1/1", roleHits.Load(), handlerHits.Load())
	}
}

// TestAdminRouteGroup_Register_NoHandlersMountsNothing — a Register call with
// no inner handlers must still produce a usable router (returning 404 to all
// /admin/* paths). This is the "boots without admin services configured" path.
func TestAdminRouteGroup_Register_NoHandlersMountsNothing(t *testing.T) {
	group := &adminRouteGroup{
		identityMW: nil, roleMW: nil, rateMW: nil, auditMW: nil,
		handlers: nil,
	}
	r := chi.NewRouter()
	group.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/admin/anything", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("got %d want 404", rr.Code)
	}
}
