// Package resume is the feature-slice root for the resume domain.
//
// NOTE: this ProviderSet documents the intended composition only — it is
// never fed through Wire's codegen; see apps/api/CLAUDE.md.
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
