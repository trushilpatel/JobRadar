package scan

import (
	"github.com/google/wire"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// ProviderSet is the Wire provider set for the scan domain.
var ProviderSet = wire.NewSet(
	svc.NewRestRouter,
	svc.NewRestService,
	svc.NewValidator,
	svc.NewService,
	svc.NewRepositoryService,
	svc.NewQueueStore,
	svc.NewLogStore,
)
