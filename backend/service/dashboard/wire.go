package dashboard

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
)

// ProviderSet is the Wire provider set for the dashboard domain.
var ProviderSet = wire.NewSet(
	services.NewDashboardRestService,
	services.NewDashboardValidationService,
	services.NewDashboardImplService,
	services.NewUserDashboardStore,
	services.NewUserDashboardService,
	services.NewPipelineStore,
	services.NewPipelineService,
)
