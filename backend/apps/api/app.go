package main

import (
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/service/application"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/dashboard"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// App holds all domain handlers wired together.
// Dependencies are injected by Wire (or manually in tests/main.go).
//
// Phase 1 multi-user services (user / role / audit) and the /api/me handler
// are not wired through this struct — they are constructed in main.go via
// buildMultiUser so the runtime-built middleware (RequireIdentity, RateLimit)
// can be threaded into the user RestRouter without forcing those middleware
// dependencies through the wire graph.
type App struct {
	companyHandler       *companysvc.RestRouter
	scanHandler          *scansvc.RestRouter
	adapterConfigHandler *scansvc.AdapterConfigRestRouter
	jobHandler           *jobsvc.RestRouter
	applicationHandler   *application.Handler
	dashboardHandler     *dashboard.Handler
	insightsHandler      *insightssvc.RestRouter
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
) *App {
	return &App{
		companyHandler:       companyHandler,
		scanHandler:          scanHandler,
		adapterConfigHandler: adapterConfigHandler,
		jobHandler:           jobHandler,
		applicationHandler:   applicationHandler,
		dashboardHandler:     dashboardHandler,
		insightsHandler:      insightsHandler,
	}
}

// Handlers returns all route groups for mounting onto the router.
func (a *App) Handlers() []handler.RouteGroup {
	return []handler.RouteGroup{
		a.companyHandler,
		a.scanHandler,
		a.adapterConfigHandler,
		a.jobHandler,
		a.applicationHandler,
		a.dashboardHandler,
		a.insightsHandler,
	}
}
