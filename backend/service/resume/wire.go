package resume

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/resume/services"
)

var ProviderSet = wire.NewSet(
	services.NewRepositoryService,
	services.NewImplService,
	services.NewValidationService,
	services.NewRestService,
	services.NewRestRouter,
)
