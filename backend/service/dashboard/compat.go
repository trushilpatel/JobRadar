package dashboard

import (
	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/service/application"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// Handler is a compatibility alias for DashboardRestService.
type Handler = services.DashboardRestService

// Validator is a compatibility alias for DashboardValidationService.
type Validator = services.DashboardValidationService

// Service is a compatibility alias for DashboardService.
type Service = services.DashboardService

// JobCountReader is a compatibility alias for the interface from services.
type JobCountReader = services.JobCountReader

// StatsView is a compatibility alias for dto.StatsView.
type StatsView = dto.StatsView

// ActivityView is a compatibility alias for dto.ActivityView.
type ActivityView = dto.ActivityView

// NewValidator constructs a Validator from the services package.
func NewValidator() Validator {
	return services.NewDashboardValidationService()
}

// NewHandler wraps NewDashboardRestService for backward compatibility.
func NewHandler(v Validator, svc Service, userSvc services.UserDashboardService, pipelineSvc services.PipelineService) *Handler {
	return services.NewDashboardRestService(v, svc, userSvc, pipelineSvc)
}

// NewService constructs a Service implementation.
func NewService(scanLog scansvc.ScanLogReader, insightsSvc insightssvc.Service, jobCounts JobCountReader) services.DashboardService {
	return services.NewDashboardImplService(scanLog, insightsSvc, jobCounts)
}

// NewUserDashboardStore constructs a UserDashboardStore from the services package.
func NewUserDashboardStore(db *gorm.DB) services.UserDashboardStore {
	return services.NewUserDashboardStore(db)
}

// NewUserDashboardService constructs a UserDashboardService.
func NewUserDashboardService(store services.UserDashboardStore, appStore application.Store) services.UserDashboardService {
	return services.NewUserDashboardService(store, appStore)
}

// NewPipelineStore constructs a PipelineStore.
func NewPipelineStore(db *gorm.DB) services.PipelineStore {
	return services.NewPipelineStore(db)
}

// NewPipelineService constructs a PipelineService.
func NewPipelineService(store services.PipelineStore, lag kafka.LagReader) services.PipelineService {
	return services.NewPipelineService(store, lag)
}
