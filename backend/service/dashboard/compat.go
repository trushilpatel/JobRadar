package dashboard

import (
	"gorm.io/gorm"

	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// Handler is a compatibility alias for RestRouter.
type Handler = services.RestRouter

// Service is a compatibility alias for DashboardService.
type Service = services.DashboardService

// JobCountReader is a compatibility alias for the interface from services.
type JobCountReader = services.JobCountReader

// StatsView is a compatibility alias for dto.StatsView.
type StatsView = dto.StatsView

// ActivityView is a compatibility alias for dto.ActivityView.
type ActivityView = dto.ActivityView

// NewService constructs a Service implementation.
func NewService(scanLog scansvc.ScanLogReader, insightsSvc insightssvc.InsightsService, jobCounts JobCountReader) services.DashboardService {
	return services.NewDashboardImplService(scanLog, insightsSvc, jobCounts)
}

// NewUserDashboardStore constructs a UserDashboardStore from the services package.
func NewUserDashboardStore(db *gorm.DB) services.UserDashboardStore {
	return services.NewUserDashboardStore(db)
}

// NewUserDashboardService constructs a UserDashboardService.
func NewUserDashboardService(store services.UserDashboardStore, appStore services.ApplicationListReader) services.UserDashboardService {
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
