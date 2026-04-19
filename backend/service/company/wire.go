package company

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/company/services"
)

// ProviderSet is the Wire provider set for the company domain.
var ProviderSet = wire.NewSet(
	services.NewRestRouter,
	services.NewRestService,
	services.NewValidator,
	services.NewService,
	services.NewRepositoryService,
	services.NewStore,
)
