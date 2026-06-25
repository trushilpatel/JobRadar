package insights

import (
	"github.com/google/wire"
	svc "github.com/trushilpatel/jobradar/service/insights/services"
)

// ProviderSet is the Wire provider set for the insights domain.
var ProviderSet = wire.NewSet(
	svc.NewRestRouter,
	svc.NewRestService,
	svc.NewValidationService,
	svc.NewImplService,
	svc.NewRepositoryService,
)
