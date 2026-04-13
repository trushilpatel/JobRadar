package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
)

// ── App.Handlers() ───────────────────────────────────────────────────────────

func TestApp_Handlers_ReturnsExpectedCount(t *testing.T) {
	app := &App{scanIntervalMinutes: 1440}
	if got := len(app.Handlers()); got != 5 {
		t.Fatalf("expected 5 route groups, got %d", got)
	}
}

func TestApp_Handlers_NoneAreNil(t *testing.T) {
	app := &App{scanIntervalMinutes: 1440}
	for i, g := range app.Handlers() {
		if g == nil {
			t.Errorf("handler[%d] is nil", i)
		}
	}
}

func TestApp_Handlers_AllImplementRouteGroup(t *testing.T) {
	app := &App{scanIntervalMinutes: 1440}
	for i, g := range app.Handlers() {
		if _, ok := g.(handler.RouteGroup); !ok {
			t.Errorf("handler[%d] does not implement RouteGroup", i)
		}
	}
}

// ── Full router coverage ──────────────────────────────────────────────────────

// TestRouter_AllRoutesMounted builds the real router via App.Handlers() and
// uses chi.Walk to verify every expected API path is registered.
// No stores are invoked — chi.Walk only inspects the route tree.
func TestRouter_AllRoutesMounted(t *testing.T) {
	app := &App{scanIntervalMinutes: 1440}

	r := chi.NewRouter()
	for _, g := range app.Handlers() {
		g.Register(r)
	}

	// Collect all mounted routes via chi.Walk (no handler invocation)
	mounted := map[string]bool{}
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+":"+strings.TrimSuffix(route, "/")] = true
		return nil
	})

	expected := []struct{ method, route string }{
		// Jobs
		{http.MethodGet, "/jobs"},
		{http.MethodGet, "/jobs/{id}"},
		{http.MethodDelete, "/jobs/{id}"},
		{http.MethodPatch, "/jobs/{id}/save"},
		{http.MethodPatch, "/jobs/{id}/status"},
		{http.MethodPatch, "/jobs/{id}/notes"},
		// Applications
		{http.MethodGet, "/jobs/{id}/applications"},
		{http.MethodPost, "/jobs/{id}/applications"},
		{http.MethodPatch, "/jobs/{id}/applications/{app_id}/stage"},
		{http.MethodPatch, "/jobs/{id}/applications/{app_id}/note"},
		{http.MethodGet, "/applications"},
		// Companies
		{http.MethodGet, "/companies"},
		{http.MethodPost, "/companies"},
		{http.MethodPatch, "/companies/active"},
		{http.MethodGet, "/companies/queue"},
		{http.MethodDelete, "/companies/queue/{entry_id}"},
		{http.MethodPost, "/companies/queue/{entry_id}/move-up"},
		{http.MethodPost, "/companies/queue/{entry_id}/move-down"},
		{http.MethodPatch, "/companies/{id}"},
		{http.MethodDelete, "/companies/{id}"},
		{http.MethodGet, "/companies/{id}/scan-stats"},
		{http.MethodPost, "/companies/{id}/scan"},
		// Dashboard
		{http.MethodGet, "/dashboard/stats"},
		{http.MethodGet, "/dashboard/activity"},
		// Insights
		{http.MethodGet, "/insights/tech-stack"},
		{http.MethodGet, "/insights/company-activity"},
		{http.MethodGet, "/insights/funnel"},
		{http.MethodGet, "/insights/location"},
		{http.MethodGet, "/insights/sponsorship"},
		{http.MethodGet, "/insights/velocity"},
		{http.MethodGet, "/insights/role-categories"},
	}

	for _, tc := range expected {
		key := tc.method + ":" + tc.route
		if !mounted[key] {
			t.Errorf("route not mounted: %s %s", tc.method, tc.route)
		}
	}
}

// TestRouter_OperationalEndpointsSeparateFromHandlers verifies that the
// operational endpoints (/health, /ready, /metrics) are NOT in App.Handlers() —
// they are mounted directly in NewRouter() and should not be owned by any group.
func TestRouter_OperationalEndpointsSeparateFromHandlers(t *testing.T) {
	app := &App{scanIntervalMinutes: 1440}

	r := chi.NewRouter()
	for _, g := range app.Handlers() {
		g.Register(r)
	}

	operational := []struct{ method, route string }{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/ready"},
		{http.MethodGet, "/metrics"},
	}

	mounted := map[string]bool{}
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+":"+strings.TrimSuffix(route, "/")] = true
		return nil
	})

	for _, tc := range operational {
		key := tc.method + ":" + tc.route
		if mounted[key] {
			t.Errorf("operational endpoint %s %s should not be in App.Handlers()", tc.method, tc.route)
		}
	}
}
