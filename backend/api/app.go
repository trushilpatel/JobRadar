package api

import (
	"github.com/trushilpatel/jobradar/api/handler"
	"github.com/trushilpatel/jobradar/service/application"
	"github.com/trushilpatel/jobradar/service/company"
	"github.com/trushilpatel/jobradar/service/dashboard"
	"github.com/trushilpatel/jobradar/service/insights"
	"github.com/trushilpatel/jobradar/service/job"
	"github.com/trushilpatel/jobradar/service/scan"
)

// App holds all domain handlers wired together.
// Dependencies are injected by Wire (or manually in tests/main.go).
type App struct {
	companyHandler     *company.Handler
	scanHandler        *scan.Handler
	jobHandler         *job.Handler
	applicationHandler *application.Handler
	dashboardHandler   *dashboard.Handler
	insightsHandler    *insights.Handler
}

// NewApp constructs the App from fully-wired domain handlers.
// Wire generates the call to this function in wire_gen.go.
func NewApp(
	companyHandler *company.Handler,
	scanHandler *scan.Handler,
	jobHandler *job.Handler,
	applicationHandler *application.Handler,
	dashboardHandler *dashboard.Handler,
	insightsHandler *insights.Handler,
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
