package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// routeCase is a method+path pair to exercise against a registered router.
type routeCase struct {
	method string
	path   string
}

// assertRoutesMounted registers g onto a fresh chi router, fires one request
// per case, and fails if any route returns 404 (not mounted) or 405 (wrong method).
func assertRoutesMounted(t *testing.T, g RouteGroup, cases []routeCase) {
	t.Helper()
	r := chi.NewRouter()
	g.Register(r)

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Errorf("route not mounted: %s %s → 404", tc.method, tc.path)
		}
		if w.Code == http.StatusMethodNotAllowed {
			t.Errorf("wrong method for route: %s %s → 405", tc.method, tc.path)
		}
	}
}

// ── Interface compliance (compile-time) ───────────────────────────────────────

// If any handler is missing Register(), this function will not compile.
func TestRouteGroup_InterfaceCompliance(t *testing.T) {
	var _ RouteGroup = &JobHandler{}
	var _ RouteGroup = &ApplicationHandler{}
	var _ RouteGroup = &CompanyHandler{}
	var _ RouteGroup = &DashboardHandler{}
	var _ RouteGroup = &InsightsHandler{}
}

// ── JobHandler.Register ───────────────────────────────────────────────────────

func TestJobHandler_Register_AllRoutesMounted(t *testing.T) {
	h := newJobHandler(newFakeJobStore(jobWithID("j1", "new")))
	assertRoutesMounted(t, h, []routeCase{
		{http.MethodGet, "/jobs"},
		{http.MethodGet, "/jobs/j1"},
		{http.MethodDelete, "/jobs/j1"},
		{http.MethodPatch, "/jobs/j1/save"},
		{http.MethodPatch, "/jobs/j1/status"},
		{http.MethodPatch, "/jobs/j1/notes"},
	})
}

func TestJobHandler_Register_WrongMethodReturns405(t *testing.T) {
	h := newJobHandler(newFakeJobStore())
	r := chi.NewRouter()
	h.Register(r)

	// POST /jobs is not registered — should be 405 (method not allowed on known path)
	// but chi may return 405 only for paths with other methods registered.
	// At minimum, verify the handler is set up without panicking.
	req := httptest.NewRequest(http.MethodPost, "/jobs/j1/save", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// PATCH /jobs/{id}/save is registered, POST is not — expect 405
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST /jobs/{id}/save, got %d", w.Code)
	}
}

// ── ApplicationHandler.Register ──────────────────────────────────────────────

func TestApplicationHandler_Register_AllRoutesMounted(t *testing.T) {
	h := newAppHandler(&fakeApplicationStore{})
	assertRoutesMounted(t, h, []routeCase{
		{http.MethodGet, "/jobs/j1/applications"},
		{http.MethodPost, "/jobs/j1/applications"},
		{http.MethodPatch, "/jobs/j1/applications/1/stage"},
		{http.MethodPatch, "/jobs/j1/applications/1/note"},
		{http.MethodGet, "/applications"},
	})
}

// ── CompanyHandler.Register ───────────────────────────────────────────────────

func TestCompanyHandler_Register_AllRoutesMounted(t *testing.T) {
	h := newCompanyHandler(newFakeCompanyStore(testCompany("c1", nil)), &fakeScanQueueStore{})
	assertRoutesMounted(t, h, []routeCase{
		{http.MethodGet, "/companies"},
		{http.MethodPost, "/companies"},
		{http.MethodPatch, "/companies/active"},
		{http.MethodGet, "/companies/queue"},
		{http.MethodDelete, "/companies/queue/entry-1"},
		{http.MethodPost, "/companies/queue/entry-1/move-up"},
		{http.MethodPost, "/companies/queue/entry-1/move-down"},
		{http.MethodPatch, "/companies/c1"},
		{http.MethodGet, "/companies/c1/scan-stats"},
		{http.MethodPost, "/companies/c1/scan"},
		{http.MethodDelete, "/companies/c1"}, // last: removes c1 from fake store
	})
}

// ── DashboardHandler.Register ─────────────────────────────────────────────────

func TestDashboardHandler_Register_AllRoutesMounted(t *testing.T) {
	h := NewDashboardHandler(newFakeJobStore(), &fakeScanLogReader{}, &fakeInsightsReader{})
	assertRoutesMounted(t, h, []routeCase{
		{http.MethodGet, "/dashboard/stats"},
		{http.MethodGet, "/dashboard/activity"},
	})
}

// ── InsightsHandler.Register ──────────────────────────────────────────────────

func TestInsightsHandler_Register_AllRoutesMounted(t *testing.T) {
	h := NewInsightsHandler(&fakeInsightsReader{})
	assertRoutesMounted(t, h, []routeCase{
		{http.MethodGet, "/insights/tech-stack"},
		{http.MethodGet, "/insights/company-activity"},
		{http.MethodGet, "/insights/funnel"},
		{http.MethodGet, "/insights/location"},
		{http.MethodGet, "/insights/sponsorship"},
		{http.MethodGet, "/insights/velocity"},
		{http.MethodGet, "/insights/role-categories"},
	})
}
