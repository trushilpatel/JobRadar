package api

import (
	"github.com/trushilpatel/jobradar/api/handler"
	"github.com/trushilpatel/jobradar/service/application"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/dashboard"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// App holds all domain handlers wired together.
// Dependencies are injected by Wire (or manually in tests/main.go).
type App struct {
	companyHandler     *companysvc.RestRouter
	scanHandler        *scansvc.RestRouter
	jobHandler         *jobsvc.RestRouter
	applicationHandler *application.Handler
	dashboardHandler   *dashboard.Handler
	insightsHandler    *insightssvc.RestRouter
}

// NewApp constructs the App from fully-wired domain handlers.
// Wire generates the call to this function in wire_gen.go.
func NewApp(
	companyHandler *companysvc.RestRouter,
	scanHandler *scansvc.RestRouter,
	jobHandler *jobsvc.RestRouter,
	applicationHandler *application.Handler,
	dashboardHandler *dashboard.Handler,
	insightsHandler *insightssvc.RestRouter,
) *App {
	return &App{
		companyHandler:     companyHandler,
		scanHandler:        scanHandler,
		jobHandler:         jobHandler,
		applicationHandler: applicationHandler,
		dashboardHandler:   dashboardHandler,
		insightsHandler:    insightsHandler,
	}
}

// Handlers returns all route groups for mounting onto the router.
func (a *App) Handlers() []handler.RouteGroup {
	return []handler.RouteGroup{
		a.companyHandler,
		a.scanHandler,
		a.jobHandler,
		a.applicationHandler,
		a.dashboardHandler,
		a.insightsHandler,
	}
}
