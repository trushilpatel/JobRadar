package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	appdto "github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	appsvc "github.com/trushilpatel/jobradar/service/application/services"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/dashboard"
	dashboarddto "github.com/trushilpatel/jobradar/service/dashboard/dto"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
	insightsdto "github.com/trushilpatel/jobradar/service/insights/dto"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scandto "github.com/trushilpatel/jobradar/service/scan/dto"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// buildTestApp creates an App with minimal fakes — only route registration is tested.
func buildTestApp() *App {
	fakeCompany := &fakeCompanyService{}
	companyRest := companysvc.NewRestService(fakeCompany)
	companyH := companysvc.NewRestRouter(companyRest)

	fakeScan := &fakeScanService{}
	scanRest := scansvc.NewRestService(fakeScan)
	scanH := scansvc.NewRestRouter(scanRest)

	fakeAdapterCfg := &fakeAdapterConfigService{}
	adapterCfgValidation := scansvc.NewAdapterConfigValidator(fakeAdapterCfg)
	adapterCfgRest := scansvc.NewAdapterConfigRestService(adapterCfgValidation)
	adapterCfgH := scansvc.NewAdapterConfigRestRouter(adapterCfgRest)

	fakeJob := &fakeJobService{}
	jobRest := jobsvc.NewRestService(fakeJob)
	jobH := jobsvc.NewRestRouter(jobRest)

	fakeApp := &fakeAppService{}
	appImpl := appsvc.NewApplicationImplService(fakeApp, fakeJob)
	appValidation := appsvc.NewApplicationValidationService(appImpl)
	appRest := appsvc.NewApplicationRestService(appValidation)
	appH := appsvc.NewApplicationRestRouter(appRest)

	dashRest := dashboardsvc.NewDashboardRestService(&fakeDashboardService{}, &fakeUserDashboardService{})
	dashH := dashboardsvc.NewRestRouter(dashRest)

	fakeInsights := &fakeInsightsService{}
	insightsValidation := insightssvc.NewValidationService(fakeInsights)
	insightsRest := insightssvc.NewRestService(insightsValidation)
	insightsH := insightssvc.NewRestRouter(insightsRest)

	return NewApp(companyH, scanH, adapterCfgH, jobH, appH, dashH, insightsH, &fakePipelineService{}, nil)
}

func TestApp_Handlers_ReturnsExpectedCount(t *testing.T) {
	app := buildTestApp()
	if got := len(app.Handlers()); got != 5 {
		t.Fatalf("expected 5 route groups, got %d", got)
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
	r.Route("/api", func(api chi.Router) {
		for _, g := range app.Handlers() {
			g.Register(api)
		}
	})

	mounted := map[string]bool{}
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+":"+strings.TrimSuffix(route, "/")] = true
		return nil
	})

	expected := []struct{ method, route string }{
		// Jobs
		{http.MethodGet, "/api/jobs"},
		{http.MethodGet, "/api/jobs/{id}"},
		{http.MethodDelete, "/api/jobs/{id}"},
		{http.MethodPatch, "/api/jobs/{id}/save"},
		{http.MethodPatch, "/api/jobs/{id}/status"},
		{http.MethodPatch, "/api/jobs/{id}/notes"},
		// Applications
		{http.MethodGet, "/api/jobs/{id}/applications"},
		{http.MethodPost, "/api/jobs/{id}/applications"},
		{http.MethodPatch, "/api/jobs/{id}/applications/{app_id}/stage"},
		{http.MethodPatch, "/api/jobs/{id}/applications/{app_id}/note"},
		{http.MethodGet, "/api/applications"},
		// Companies (public read-only; mutations live under /admin)
		{http.MethodGet, "/api/companies"},
		{http.MethodGet, "/api/companies/{id}"},
		// Scan operations
		{http.MethodGet, "/api/companies/queue"},
		{http.MethodDelete, "/api/companies/queue/{entry_id}"},
		{http.MethodGet, "/api/companies/{id}/scan-stats"},
		{http.MethodPost, "/api/companies/{id}/scan"},
		// Dashboard (user-facing panels; pipeline stats live under /admin)
		{http.MethodGet, "/api/dashboard/stats"},
		{http.MethodGet, "/api/dashboard/activity"},
		{http.MethodGet, "/api/dashboard/progress"},
		{http.MethodGet, "/api/dashboard/market"},
		{http.MethodGet, "/api/dashboard/heatmap"},
		{http.MethodGet, "/api/dashboard/velocity"},
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
	r.Route("/api", func(api chi.Router) {
		for _, g := range app.Handlers() {
			g.Register(api)
		}
	})

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

func (f *fakeCompanyService) ListPublic(_ context.Context, _ companydto.CompanyFilter, _ int) (companydto.CompanyPublicListResult, error) {
	return companydto.CompanyPublicListResult{}, nil
}
func (f *fakeCompanyService) GetPublic(_ context.Context, _ string) (*companydto.CompanyPublicView, error) {
	return nil, nil
}

type fakeScanService struct{}

func (f *fakeScanService) TriggerScan(_ context.Context, _ string) error { return nil }
func (f *fakeScanService) TriggerScanAll(_ context.Context) (int, error) { return 0, nil }
func (f *fakeScanService) GetStats(_ context.Context, _ string) (*scandto.ScanStatsView, error) {
	return nil, nil
}
func (f *fakeScanService) ListQueue(_ context.Context, _ scandto.QueueFilter) ([]scandto.QueueEntryView, error) {
	return nil, nil
}
func (f *fakeScanService) RemoveFromQueue(_ context.Context, _ string) error { return nil }
func (f *fakeScanService) EnqueueDue(_ context.Context, _ int) (scansvc.SchedulerCycleResult, error) {
	return scansvc.SchedulerCycleResult{}, nil
}
func (f *fakeScanService) ResetStuck(_ context.Context, _ time.Duration) (int, error) {
	return 0, nil
}
func (f *fakeScanService) PurgeInactive(_ context.Context) (int, error) { return 0, nil }
func (f *fakeScanService) GetAnalytics(_ context.Context, _ string) (*scandto.CompanyAnalytics, error) {
	return &scandto.CompanyAnalytics{}, nil
}

type fakeJobService struct{}

func (f *fakeJobService) List(_ context.Context, _ jobdto.JobFilter, _ string, _ int) (jobdto.PageResult, error) {
	return jobdto.PageResult{}, nil
}
func (f *fakeJobService) Get(_ context.Context, _ string) (*jobdto.JobView, error) { return nil, nil }
func (f *fakeJobService) Dismiss(_ context.Context, _ string, _, _ string) error   { return nil }
func (f *fakeJobService) SetSaved(_ context.Context, _ string, _ bool) error       { return nil }
func (f *fakeJobService) SetStatus(_ context.Context, _, _ string) error           { return nil }
func (f *fakeJobService) SetNotes(_ context.Context, _, _ string) error            { return nil }
func (f *fakeJobService) BulkDismiss(_ context.Context, _ []string, _, _ string) error {
	return nil
}
func (f *fakeJobService) GetCounts(_ context.Context) (jobdto.Counts, error) {
	return jobdto.Counts{}, nil
}
func (f *fakeJobService) GetCompanies(_ context.Context) ([]string, error) { return nil, nil }

type fakeAppService struct{}

func (f *fakeAppService) ListByJob(_ context.Context, _, _ string) ([]appmodel.JobApplication, error) {
	return nil, nil
}
func (f *fakeAppService) ListAll(_ context.Context, _ string, _, _ int) ([]appsvc.ApplicationWithJob, int, error) {
	return nil, 0, nil
}
func (f *fakeAppService) ListActive(_ context.Context, _ string) ([]appsvc.ApplicationWithJob, error) {
	return nil, nil
}
func (f *fakeAppService) Create(_ context.Context, _, _ string, _ appdto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	return nil, nil
}
func (f *fakeAppService) UpdateStage(_ context.Context, _, _ string, _ appdto.UpdateStageInput) error {
	return nil
}
func (f *fakeAppService) UpdateNote(_ context.Context, _, _ string, _ appdto.UpdateNoteInput) error {
	return nil
}

type fakeDashboardService struct{}

func (f *fakeDashboardService) GetStats(_ context.Context) (*dashboard.StatsView, error) {
	return nil, nil
}
func (f *fakeDashboardService) GetActivity(_ context.Context, _ int) ([]dashboard.ActivityView, error) {
	return nil, nil
}

type fakeUserDashboardService struct{}

func (f *fakeUserDashboardService) GetProgressStats(_ context.Context) (*dashboarddto.ProgressStats, error) {
	return nil, nil
}
func (f *fakeUserDashboardService) GetMarketStats(_ context.Context) (*dashboarddto.MarketStats, error) {
	return nil, nil
}
func (f *fakeUserDashboardService) GetHeatmap(_ context.Context) ([]dashboarddto.WeeklyCount, error) {
	return nil, nil
}
func (f *fakeUserDashboardService) GetInterviews(_ context.Context) ([]dashboarddto.InterviewRow, error) {
	return nil, nil
}
func (f *fakeUserDashboardService) GetVelocity(_ context.Context, _ int) ([]dashboarddto.CompanyVelocityRow, error) {
	return nil, nil
}
func (f *fakeUserDashboardService) GetTechTrends(_ context.Context) (*dashboarddto.TechTrends, error) {
	return nil, nil
}

type fakePipelineService struct{}

func (f *fakePipelineService) GetPipelineStats(_ context.Context) (*dashboarddto.PipelineStats, error) {
	return nil, nil
}

func (f *fakePipelineService) TriggerRetry(_ context.Context, _, _ string) (int64, error) {
	return 0, nil
}

type fakeInsightsService struct{}

func (f *fakeInsightsService) TechStack(_ context.Context) ([]insightsdto.TechCount, error) {
	return nil, nil
}
func (f *fakeInsightsService) CompanyActivity(_ context.Context) ([]insightsdto.CompanyActivityItem, error) {
	return nil, nil
}
func (f *fakeInsightsService) Funnel(_ context.Context) ([]insightsdto.FunnelStep, error) {
	return nil, nil
}
func (f *fakeInsightsService) Location(_ context.Context) (insightsdto.LocationBreakdown, error) {
	return insightsdto.LocationBreakdown{}, nil
}
func (f *fakeInsightsService) Sponsorship(_ context.Context, _ insightsdto.InsightsFilter) ([]insightsdto.SponsorshipDay, error) {
	return nil, nil
}
func (f *fakeInsightsService) Velocity(_ context.Context) ([]insightsdto.CompanyVelocity, error) {
	return nil, nil
}
func (f *fakeInsightsService) RoleCategories(_ context.Context) ([]insightsdto.RoleCategoryCount, error) {
	return nil, nil
}
func (f *fakeInsightsService) GetPipelineCounts(_ context.Context) (insightsdto.PipelineCounts, error) {
	return insightsdto.PipelineCounts{}, nil
}

type fakeAdapterConfigService struct{}

func (f *fakeAdapterConfigService) ListAdapterConfigs(_ context.Context) ([]scandto.AdapterConfigDTO, error) {
	return nil, nil
}
func (f *fakeAdapterConfigService) UpdateAdapterConfig(_ context.Context, _ string, _ scandto.UpdateAdapterConfigInput) (scandto.AdapterConfigDTO, error) {
	return scandto.AdapterConfigDTO{}, nil
}
func (f *fakeAdapterConfigService) GetEffectiveScanConfig(_ context.Context, _ string) (scandto.EffectiveScanConfigDTO, error) {
	return scandto.EffectiveScanConfigDTO{}, nil
}
func (f *fakeAdapterConfigService) UpdateScanIntervalOverride(_ context.Context, _ string, _ *int) error {
	return nil
}

// Ensure fakes implement interfaces.
var _ companysvc.CompanyService = (*fakeCompanyService)(nil)
var _ scansvc.Service = (*fakeScanService)(nil)
var _ scansvc.AdapterConfigService = (*fakeAdapterConfigService)(nil)
var _ jobsvc.JobService = (*fakeJobService)(nil)
var _ appsvc.ApplicationRepository = (*fakeAppService)(nil)
var _ dashboard.Service = (*fakeDashboardService)(nil)
var _ dashboardsvc.UserDashboardService = (*fakeUserDashboardService)(nil)
var _ dashboardsvc.PipelineService = (*fakePipelineService)(nil)
var _ insightssvc.InsightsService = (*fakeInsightsService)(nil)
