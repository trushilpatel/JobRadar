// Package pipeline is the feature-slice root for the pipeline domain. The
// wire provider set here composes the decorator chain in the canonical
// order:
//
//	Repository → Impl → Validation → Rest → RestRouter
//
// Wire builds the chain right-to-left from the constructor signatures. Each
// constructor returns the same PipelineService interface so composition is
// straightforward.
//
// NOTE: apps/api/wire.go's wire.Build graph currently fails to regenerate
// (pre-existing multiple-bindings conflicts across company/scan/job/
// application predate this slice) — apps/api/wire_gen.go is hand-maintained
// there. This ProviderSet documents the intended composition; the actual
// wiring for this slice lives alongside the ats wiring in wire_gen.go.
package pipeline

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/pipeline/services"
)

// ProviderSet is the Wire provider set for the pipeline domain.
var ProviderSet = wire.NewSet(
	services.NewPipelineStore,
	services.NewPipelineRepositoryService,
	services.NewPipelineImplService,
	services.NewPipelineValidationService,
	services.NewPipelineRestService,
	services.NewPipelineRestRouter,
)
