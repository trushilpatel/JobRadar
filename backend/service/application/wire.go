package application

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/application/services"
)

// ProviderSet is the Wire provider set for the application domain.
var ProviderSet = wire.NewSet(
	services.NewApplicationRestHandler,
	services.NewApplicationValidationService,
	services.NewApplicationImplService,
	services.NewApplicationRepositoryService,
)
