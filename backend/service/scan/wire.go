package scan

import (
	"github.com/google/wire"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// ProviderSet is the Wire provider set for the scan domain.
var ProviderSet = wire.NewSet(
	svc.NewRestService,
	svc.NewValidator,
	svc.NewService,
	svc.NewQueueStore,
	svc.NewLogStore,
)
