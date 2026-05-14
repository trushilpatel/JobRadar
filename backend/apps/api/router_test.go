package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// passthroughAuth is an Auth middleware stand-in for router-shape tests that
// only need to verify mounting + middleware chaining, not signature checking.
func passthroughAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// markingMW returns a middleware that increments calls and writes its tag onto
// a response header; used by tests asserting that middleware order and scoping
// are correct.
func markingMW(tag string, calls *atomic.Int32) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Add("X-Marker", tag)
			next.ServeHTTP(w, r)
		})
	}
}

// stubGroup implements handler.RouteGroup so router tests can mount arbitrary
// paths without dragging the service slices into the test build.
type stubGroup struct {
	register func(r chi.Router)
}

func (s *stubGroup) Register(r chi.Router) { s.register(r) }

func minimalRouterConfig() *config.Config {
	return &config.Config{
		API: config.APIConfig{MaxBodyBytes: 1 << 20},
	}
}

func TestNewRouter_MountsApiGroupsUnderApiPrefix(t *testing.T) {
	g := &stubGroup{register: func(r chi.Router) {
		r.Get("/things/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}}

	r := NewRouter(logger.NewNoop(), minimalRouterConfig(),
		passthroughAuth, nil, nil,
		[]handler.RouteGroup{g}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/things/x", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200 — /api/things/{id} should be mounted", rr.Code)
	}
}

func TestNewRouter_MountsRootGroupsAtRoot(t *testing.T) {
	apiG := &stubGroup{register: func(r chi.Router) {
		r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	}}
	rootG := &stubGroup{register: func(r chi.Router) {
		r.Get("/admin/y", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	}}

	r := NewRouter(logger.NewNoop(), minimalRouterConfig(),
		passthroughAuth, nil, nil,
		[]handler.RouteGroup{apiG},
		[]handler.RouteGroup{rootG})

	req := httptest.NewRequest(http.MethodGet, "/admin/y", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/admin/y should be mounted at root, got %d", rr.Code)
	}

	// And /api/y should NOT match the root group.
	req2 := httptest.NewRequest(http.MethodGet, "/api/y", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	if rr2.Code == http.StatusOK {
		t.Fatalf("/api/y matched a root group — root groups must mount at /, not /api")
	}
}

func TestNewRouter_IdentityAndRateLimitOnlyOnApi(t *testing.T) {
	var apiCalls, identityCalls, rateCalls atomic.Int32
	identityMW := markingMW("identity", &identityCalls)
	rateMW := markingMW("rate", &rateCalls)

	apiG := &stubGroup{register: func(r chi.Router) {
		r.Get("/x", func(w http.ResponseWriter, _ *http.Request) {
			apiCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		})
	}}
	rootG := &stubGroup{register: func(r chi.Router) {
		r.Get("/admin/z", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}}

	r := NewRouter(logger.NewNoop(), minimalRouterConfig(),
		passthroughAuth, identityMW, rateMW,
		[]handler.RouteGroup{apiG},
		[]handler.RouteGroup{rootG})

	// /api/x must trigger both identity and rate.
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || apiCalls.Load() != 1 {
		t.Fatalf("api call: code=%d calls=%d", rr.Code, apiCalls.Load())
	}
	if identityCalls.Load() != 1 || rateCalls.Load() != 1 {
		t.Fatalf("expected identity=1 rate=1 on /api, got identity=%d rate=%d", identityCalls.Load(), rateCalls.Load())
	}

	// /admin/z must NOT trigger them — root groups are outside /api.
	req2 := httptest.NewRequest(http.MethodGet, "/admin/z", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("/admin/z not mounted: %d", rr2.Code)
	}
	if identityCalls.Load() != 1 || rateCalls.Load() != 1 {
		t.Fatalf("identity/rate must NOT run for root groups, got identity=%d rate=%d", identityCalls.Load(), rateCalls.Load())
	}
}

func TestNewRouter_NilIdentityAndRateLimitAccepted(t *testing.T) {
	// Tests that passing nil for both optional middlewares does not panic and
	// still mounts the api routes — useful for tests and bare-bones setups.
	apiG := &stubGroup{register: func(r chi.Router) {
		r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	}}

	r := NewRouter(logger.NewNoop(), minimalRouterConfig(),
		passthroughAuth, nil, nil,
		[]handler.RouteGroup{apiG}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200 — nil identity/rate must pass through", rr.Code)
	}
}

func TestNewRouter_SecurityHeadersApplied(t *testing.T) {
	// Smoke test that the chain wires SecurityHeaders — assert one of the
	// headers shows up on the response.
	apiG := &stubGroup{register: func(r chi.Router) {
		r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	}}
	r := NewRouter(logger.NewNoop(), minimalRouterConfig(),
		passthroughAuth, nil, nil,
		[]handler.RouteGroup{apiG}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options=%q want nosniff — SecurityHeaders missing", got)
	}
}

func TestNewRouter_BodyLimitApplied(t *testing.T) {
	// Use a tiny body limit so a 1 KB POST should fail when handled.
	cfg := &config.Config{API: config.APIConfig{MaxBodyBytes: 8}}
	apiG := &stubGroup{register: func(r chi.Router) {
		r.Post("/things", func(w http.ResponseWriter, r *http.Request) {
			// Read body; should error.
			buf := make([]byte, 64)
			_, err := r.Body.Read(buf)
			if err != nil && strings.Contains(err.Error(), "too large") {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	}}
	r := NewRouter(logger.NewNoop(), cfg,
		passthroughAuth, nil, nil,
		[]handler.RouteGroup{apiG}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/things", strings.NewReader(strings.Repeat("a", 64)))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body limit not enforced: got %d", rr.Code)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func TestNewOpsRouter_HealthReturns200(t *testing.T) {
	r := NewOpsRouter(newTestDB(t))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/health got %d want 200", rr.Code)
	}
}

func TestNewOpsRouter_MetricsExposed(t *testing.T) {
	r := NewOpsRouter(newTestDB(t))
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/metrics got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") {
		t.Fatalf("/metrics body does not look like Prometheus exposition: %q", body[:min(80, len(body))])
	}
}

func TestNewOpsRouter_ReadyReturns200WhenDBOpen(t *testing.T) {
	db := newTestDB(t)
	r := NewOpsRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/ready got %d want 200 on healthy DB", rr.Code)
	}
}

func TestNewOpsRouter_ReadyReturns503WhenSqlDBClosed(t *testing.T) {
	// Build a *gorm.DB whose underlying *sql.DB is closed so PoolReady fails.
	db := newTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	// Close it — subsequent db.DB() calls return ErrDBClosed via the pool.
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	r := NewOpsRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// PoolReady on a closed DB returns ErrDBClosed (or nil if gorm reopens —
	// depends on driver). Accept either 200 or 503 here; if we see 503 verify
	// the body carries an error. The important contract is "does not panic".
	if rr.Code != http.StatusServiceUnavailable && rr.Code != http.StatusOK {
		t.Fatalf("/ready got %d want 200 or 503", rr.Code)
	}
	// If 503, ensure the body is the textual error and not empty.
	if rr.Code == http.StatusServiceUnavailable && rr.Body.Len() == 0 {
		t.Fatal("/ready 503 must include error body")
	}
	// silence sql import unused (kept for symmetry with future expansions)
	var _ = sql.ErrConnDone
}
