package main

import (
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/service/application"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	pipelinesvc "github.com/trushilpatel/jobradar/service/pipeline/services"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// App holds all domain handlers wired together.
// Dependencies are injected by Wire (or manually in tests/main.go).
type App struct {
	companyHandler      *companysvc.RestRouter
	scanHandler         *scansvc.RestRouter
	atsHandler          *atssvc.AtsRestRouter
	jobHandler          *jobsvc.RestRouter
	applicationHandler  *application.Handler
	companyAdminHandler *companysvc.AdminRestRouter
	resumeHandler       *resumesvc.RestRouter
	pipelineHandler     *pipelinesvc.PipelineRestRouter
}

// NewApp constructs the App from fully-wired domain handlers.
// Wire generates the call to this function in wire_gen.go.
func NewApp(
	companyHandler *companysvc.RestRouter,
	scanHandler *scansvc.RestRouter,
	atsHandler *atssvc.AtsRestRouter,
	jobHandler *jobsvc.RestRouter,
	applicationHandler *application.Handler,
	companyAdminHandler *companysvc.AdminRestRouter,
	resumeHandler *resumesvc.RestRouter,
	pipelineHandler *pipelinesvc.PipelineRestRouter,
) *App {
	return &App{
		companyHandler:      companyHandler,
		scanHandler:         scanHandler,
		atsHandler:          atsHandler,
		jobHandler:          jobHandler,
		applicationHandler:  applicationHandler,
		companyAdminHandler: companyAdminHandler,
		resumeHandler:       resumeHandler,
		pipelineHandler:     pipelineHandler,
	}
}

// Handlers returns all route groups for mounting onto the public router.
// ATS and company admin are excluded — they mount under /admin via buildMultiUser.
func (a *App) Handlers() []handler.RouteGroup {
	return []handler.RouteGroup{
		a.companyHandler,
		a.scanHandler,
		a.jobHandler,
		a.applicationHandler,
		a.resumeHandler,
	}
}

// AtsHandler returns the ATS router for admin mounting.
func (a *App) AtsHandler() handler.RouteGroup {
	if a.atsHandler == nil {
		return nil
	}
	return a.atsHandler
}

// CompanyAdminHandler returns the company admin router for admin mounting.
func (a *App) CompanyAdminHandler() handler.RouteGroup {
	if a.companyAdminHandler == nil {
		return nil
	}
	return a.companyAdminHandler
}

// PipelineHandler returns the pipeline router for admin mounting.
func (a *App) PipelineHandler() handler.RouteGroup {
	if a.pipelineHandler == nil {
		return nil
	}
	return a.pipelineHandler
}
