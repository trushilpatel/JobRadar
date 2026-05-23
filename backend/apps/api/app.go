package main

import (
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/service/application"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/dashboard"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// App holds all domain handlers wired together.
// Dependencies are injected by Wire (or manually in tests/main.go).
//
// Phase 2: the insights handler and pipeline service are exposed for
// mounting under /admin — see buildMultiUser in multiuser_setup.go.
type App struct {
	companyHandler       *companysvc.RestRouter
	scanHandler          *scansvc.RestRouter
	adapterConfigHandler *scansvc.AdapterConfigRestRouter
	jobHandler           *jobsvc.RestRouter
	applicationHandler   *application.Handler
	dashboardHandler     *dashboard.Handler
	// insightsHandler is NOT returned by Handlers() — it moves to /admin in
	// Phase 2 and is threaded through buildMultiUser via InsightsHandler().
	insightsHandler *insightssvc.RestRouter
	// companyAdminHandler mounts /admin/companies — threaded via CompanyAdminHandler().
	companyAdminHandler *companysvc.AdminRestRouter
	// pipelineSvc is exposed so main.go can thread it into the admin router
	// without the Wire graph needing to know about buildMultiUser.
	pipelineSvc dashboardsvc.PipelineService
}

// NewApp constructs the App from fully-wired domain handlers.
// Wire generates the call to this function in wire_gen.go.
func NewApp(
	companyHandler *companysvc.RestRouter,
	scanHandler *scansvc.RestRouter,
	adapterConfigHandler *scansvc.AdapterConfigRestRouter,
	jobHandler *jobsvc.RestRouter,
	applicationHandler *application.Handler,
	dashboardHandler *dashboard.Handler,
	insightsHandler *insightssvc.RestRouter,
	pipelineSvc dashboardsvc.PipelineService,
	companyAdminHandler *companysvc.AdminRestRouter,
) *App {
	return &App{
		companyHandler:       companyHandler,
		scanHandler:          scanHandler,
		adapterConfigHandler: adapterConfigHandler,
		jobHandler:           jobHandler,
		applicationHandler:   applicationHandler,
		dashboardHandler:     dashboardHandler,
		insightsHandler:      insightsHandler,
		pipelineSvc:          pipelineSvc,
		companyAdminHandler:  companyAdminHandler,
	}
}

// Handlers returns all route groups for mounting onto the public router.
// Insights and AdapterConfig are intentionally excluded — they are mounted
// under /admin by buildMultiUser and must not also appear on the public router.
func (a *App) Handlers() []handler.RouteGroup {
	return []handler.RouteGroup{
		a.companyHandler,
		a.scanHandler,
		a.jobHandler,
		a.applicationHandler,
		a.dashboardHandler,
	}
}

// AdapterConfigHandler returns the adapter config router for admin mounting.
func (a *App) AdapterConfigHandler() *scansvc.AdapterConfigRestRouter {
	return a.adapterConfigHandler
}

// InsightsHandler returns the insights router for admin mounting.
func (a *App) InsightsHandler() *insightssvc.RestRouter {
	return a.insightsHandler
}

// CompanyAdminHandler returns the company admin router for admin mounting.
func (a *App) CompanyAdminHandler() *companysvc.AdminRestRouter {
	return a.companyAdminHandler
}

// PipelineSvc returns the pipeline service for admin mounting.
func (a *App) PipelineSvc() dashboardsvc.PipelineService {
	return a.pipelineSvc
}
