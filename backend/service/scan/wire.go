package scan

import (
	"github.com/google/wire"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// ProviderSet is the Wire provider set for the scan domain.
//
// NOTE: this ProviderSet documents the intended composition only — it is
// never fed through Wire's codegen; see apps/api/CLAUDE.md.
var ProviderSet = wire.NewSet(
	svc.NewRestRouter,
	svc.NewRestService,
	svc.NewValidator,
	svc.NewService,
	svc.NewRepositoryService,
	svc.NewQueueStore,
	svc.NewLogStore,
)
