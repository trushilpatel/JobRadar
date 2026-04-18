package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/service/application"
	"github.com/trushilpatel/jobradar/service/company"
	"github.com/trushilpatel/jobradar/service/dashboard"
	"github.com/trushilpatel/jobradar/service/insights"
	"github.com/trushilpatel/jobradar/service/job"
	"github.com/trushilpatel/jobradar/service/scan"
)

// buildTestApp creates an App with all real validators and nil-safe fake services.
// We only need the route tree, not actual DB calls.
func buildTestApp() *App {
	companyH := company.NewHandler(company.NewValidator(), &fakeCompanyService{})
	scanH := scan.NewHandler(scan.NewValidator(), &fakeScanService{})
	jobH := job.NewHandler(job.NewValidator(), &fakeJobService{})
	appH := application.NewHandler(application.NewValidator(), &fakeAppService{})
	dashH := dashboard.NewHandler(dashboard.NewValidator(), &fakeDashboardService{}, nil, nil)
	insightsH := insights.NewHandler(insights.NewValidator(), &fakeInsightsService{})
	return NewApp(companyH, scanH, jobH, appH, dashH, insightsH)
}

func TestApp_Handlers_ReturnsExpectedCount(t *testing.T) {
	app := buildTestApp()
	if got := len(app.Handlers()); got != 6 {
		t.Fatalf("expected 6 route groups, got %d", got)
	}
}

func TestApp_Handlers_NoneAreNil(t *testing.T) {
	app := buildTestApp()
	for i, g := range app.Handlers() {
		if g == nil {
			t.Errorf("handler[%d] is nil", i)
		}
	}
}

func TestApp_Handlers_AllImplementRouteGroup(t *testing.T) {
	app := buildTestApp()
	for i, g := range app.Handlers() {
		if g == nil {
			t.Errorf("handler[%d] is nil", i)
		}
	}
}

func TestRouter_AllRoutesMounted(t *testing.T) {
	app := buildTestApp()

	r := chi.NewRouter()
	for _, g := range app.Handlers() {
		g.Register(r)
	}

	mounted := map[string]bool{}
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+":"+strings.TrimSuffix(route, "/")] = true
		return nil
	})

	expected := []struct{ method, route string }{
		{http.MethodGet, "/jobs"},
		{http.MethodGet, "/jobs/{id}"},
		{http.MethodDelete, "/jobs/{id}"},
		{http.MethodPatch, "/jobs/{id}/save"},
		{http.MethodPatch, "/jobs/{id}/status"},
		{http.MethodPatch, "/jobs/{id}/notes"},
		{http.MethodGet, "/jobs/{id}/applications"},
		{http.MethodPost, "/jobs/{id}/applications"},
		{http.MethodPatch, "/jobs/{id}/applications/{app_id}/stage"},
		{http.MethodPatch, "/jobs/{id}/applications/{app_id}/note"},
		{http.MethodGet, "/applications"},
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
		{http.MethodGet, "/dashboard/stats"},
		{http.MethodGet, "/dashboard/activity"},
		{http.MethodGet, "/dashboard/pipeline"},
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

func TestRouter_OperationalEndpointsSeparateFromHandlers(t *testing.T) {
	app := buildTestApp()

	r := chi.NewRouter()
	for _, g := range app.Handlers() {
		g.Register(r)
	}

	mounted := map[string]bool{}
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+":"+strings.TrimSuffix(route, "/")] = true
		return nil
	})

	for _, tc := range []struct{ method, route string }{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/ready"},
		{http.MethodGet, "/metrics"},
	} {
		key := tc.method + ":" + tc.route
		if mounted[key] {
			t.Errorf("operational endpoint %s %s should not be in App.Handlers()", tc.method, tc.route)
		}
	}
}

// ── Minimal fake services for route registration testing ─────────────────────

type fakeCompanyService struct{}

func (f *fakeCompanyService) List(_ context.Context, _ company.CompanyFilter, _, _ int) (company.CompanyListResult, error) {
	return company.CompanyListResult{}, nil
}
func (f *fakeCompanyService) GetAll(_ context.Context) ([]company.CompanyView, error) {
	return nil, nil
}
func (f *fakeCompanyService) Get(_ context.Context, _ string) (*model.Company, error) {
	return nil, nil
}
func (f *fakeCompanyService) Create(_ context.Context, _ company.CreateCompanyInput) (*model.Company, error) {
	return nil, nil
}
func (f *fakeCompanyService) Update(_ context.Context, _ string, _ company.UpdateCompanyInput) error {
	return nil
}
func (f *fakeCompanyService) Delete(_ context.Context, _ string) error     { return nil }
func (f *fakeCompanyService) SetAllActive(_ context.Context, _ bool) error { return nil }

type fakeScanService struct{}

func (f *fakeScanService) TriggerScan(_ context.Context, _ string) error { return nil }
func (f *fakeScanService) TriggerScanAll(_ context.Context) (int, error) { return 0, nil }
func (f *fakeScanService) GetStats(_ context.Context, _ string) (*scan.ScanStatsView, error) {
	return nil, nil
}
func (f *fakeScanService) ListQueue(_ context.Context, _ scan.QueueFilter) ([]scan.QueueEntryView, error) {
	return nil, nil
}
func (f *fakeScanService) RemoveFromQueue(_ context.Context, _ string) error { return nil }
func (f *fakeScanService) MoveUp(_ context.Context, _ string) error          { return nil }
func (f *fakeScanService) MoveDown(_ context.Context, _ string) error        { return nil }

type fakeJobService struct{}

func (f *fakeJobService) List(_ context.Context, _ job.JobFilter, _, _ int) ([]model.Job, int, error) {
	return nil, 0, nil
}
func (f *fakeJobService) Get(_ context.Context, _ string) (*model.Job, error)           { return nil, nil }
func (f *fakeJobService) GetCounts(_ context.Context) (*job.JobCounts, error)           { return nil, nil }
func (f *fakeJobService) Dismiss(_ context.Context, _ string, _ job.DismissInput) error { return nil }
func (f *fakeJobService) SetSaved(_ context.Context, _ string, _ bool) error            { return nil }
func (f *fakeJobService) SetStatus(_ context.Context, _, _ string) error                { return nil }
func (f *fakeJobService) SetNotes(_ context.Context, _, _ string) error                 { return nil }

type fakeAppService struct{}

func (f *fakeAppService) ListByJob(_ string) ([]model.JobApplication, error) { return nil, nil }
func (f *fakeAppService) ListAll(_, _ int) ([]application.ApplicationWithJob, int, error) {
	return nil, 0, nil
}
func (f *fakeAppService) Create(_ context.Context, _ string, _ application.CreateApplicationInput) (*model.JobApplication, error) {
	return nil, nil
}
func (f *fakeAppService) UpdateStage(_, _ string, _ application.UpdateStageInput) error { return nil }
func (f *fakeAppService) UpdateNote(_, _ string, _ application.UpdateNoteInput) error   { return nil }

type fakeDashboardService struct{}

func (f *fakeDashboardService) GetStats(_ context.Context) (*dashboard.StatsView, error) {
	return nil, nil
}
func (f *fakeDashboardService) GetActivity(_ context.Context, _ int) ([]dashboard.ActivityView, error) {
	return nil, nil
}

type fakeInsightsService struct{}

func (f *fakeInsightsService) TechStack(_ context.Context) (interface{}, error) { return nil, nil }
func (f *fakeInsightsService) CompanyActivity(_ context.Context) (interface{}, error) {
	return nil, nil
}
func (f *fakeInsightsService) Funnel(_ context.Context) (interface{}, error)   { return nil, nil }
func (f *fakeInsightsService) Location(_ context.Context) (interface{}, error) { return nil, nil }
func (f *fakeInsightsService) Sponsorship(_ context.Context, _ insights.InsightsFilter) (interface{}, error) {
	return nil, nil
}
func (f *fakeInsightsService) Velocity(_ context.Context) (interface{}, error)       { return nil, nil }
func (f *fakeInsightsService) RoleCategories(_ context.Context) (interface{}, error) { return nil, nil }
func (f *fakeInsightsService) GetPipelineCounts(_ context.Context) (insights.PipelineCounts, error) {
	return insights.PipelineCounts{}, nil
}

// Ensure fakes implement interfaces.
var _ company.Service = (*fakeCompanyService)(nil)
var _ scan.Service = (*fakeScanService)(nil)
var _ job.Service = (*fakeJobService)(nil)
var _ application.Service = (*fakeAppService)(nil)
var _ dashboard.Service = (*fakeDashboardService)(nil)
var _ insights.Service = (*fakeInsightsService)(nil)
