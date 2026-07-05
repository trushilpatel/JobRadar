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
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	resumedto "github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
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

	fakeAts := &fakeAtsService{}
	atsRest := atssvc.NewAtsRestService(fakeAts)
	atsH := atssvc.NewAtsRestRouter(atsRest)

	fakeJob := &fakeJobService{}
	jobRest := jobsvc.NewRestService(fakeJob)
	jobH := jobsvc.NewRestRouter(jobRest)

	fakeApp := &fakeAppService{}
	appImpl := appsvc.NewApplicationImplService(fakeApp, fakeJob)
	appValidation := appsvc.NewApplicationValidationService(appImpl)
	appRest := appsvc.NewApplicationRestService(appValidation)
	appH := appsvc.NewApplicationRestRouter(appRest)

	fakeResume := &fakeResumeService{}
	resumeRest := resumesvc.NewRestService(fakeResume)
	resumeH := resumesvc.NewRestRouter(resumeRest)

	return NewApp(companyH, scanH, atsH, jobH, appH, nil, resumeH, nil)
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
		// Resumes
		{http.MethodGet, "/api/resumes"},
		{http.MethodPost, "/api/resumes"},
		{http.MethodGet, "/api/resumes/{id}/download"},
		{http.MethodPatch, "/api/resumes/{id}"},
		{http.MethodDelete, "/api/resumes/{id}"},
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
func (f *fakeScanService) GetEffectiveScanConfig(_ context.Context, _ string) (scandto.EffectiveScanConfigDTO, error) {
	return scandto.EffectiveScanConfigDTO{}, nil
}
func (f *fakeScanService) UpdateScanIntervalOverride(_ context.Context, _ string, _ *int) error {
	return nil
}
func (f *fakeScanService) RetryDueJobs(_ context.Context) error                      { return nil }
func (f *fakeScanService) EscalateStuckRetryJobs(_ context.Context) (int64, error)   { return 0, nil }
func (f *fakeScanService) ReclaimStuckFetchRows(_ context.Context) (int64, error)    { return 0, nil }
func (f *fakeScanService) ReclaimStuckFilteredRows(_ context.Context) (int64, error) { return 0, nil }
func (f *fakeScanService) AutoPauseChronicFailures(_ context.Context) (int, error)   { return 0, nil }
func (f *fakeScanService) RefreshGaugeMetrics(_ context.Context) error               { return nil }

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

type fakeResumeService struct{}

func (f *fakeResumeService) List(_ context.Context, _ string) ([]resumemodel.Resume, error) {
	return nil, nil
}
func (f *fakeResumeService) Get(_ context.Context, _, _ string) (*resumemodel.Resume, error) {
	return nil, nil
}
func (f *fakeResumeService) Upload(_ context.Context, _ string, _ resumedto.UploadInput) (*resumemodel.Resume, error) {
	return nil, nil
}
func (f *fakeResumeService) Rename(_ context.Context, _, _, _ string) (*resumemodel.Resume, error) {
	return nil, nil
}
func (f *fakeResumeService) Delete(_ context.Context, _, _ string) error { return nil }
func (f *fakeResumeService) PresignDownload(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

type fakeAtsService struct{}

func (f *fakeAtsService) ListATS(_ context.Context) ([]atsdto.ATSDTO, error) {
	return nil, nil
}
func (f *fakeAtsService) GetATS(_ context.Context, _ string) (atsdto.ATSDTO, error) {
	return atsdto.ATSDTO{}, nil
}
func (f *fakeAtsService) UpdateATS(_ context.Context, _ string, _ atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	return atsdto.ATSDTO{}, nil
}

// Ensure fakes implement interfaces.
var _ companysvc.CompanyService = (*fakeCompanyService)(nil)
var _ scansvc.Service = (*fakeScanService)(nil)
var _ atssvc.AtsService = (*fakeAtsService)(nil)
var _ jobsvc.JobService = (*fakeJobService)(nil)
var _ appsvc.ApplicationRepository = (*fakeAppService)(nil)
var _ resumesvc.Service = (*fakeResumeService)(nil)
