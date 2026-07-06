// Package pipeline is the feature-slice root for the pipeline domain. The
// wire provider set here composes the decorator chain in the canonical
// order:
//
//	RestRouter → RestService → Validation → Impl → Repository
//
// Wire builds the chain from the constructor signatures regardless of listed
// order. Each constructor returns the same PipelineService interface so
// composition is straightforward.
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
	services.NewRestRouter,
	services.NewRestService,
	services.NewValidationService,
	services.NewImplService,
	services.NewRepositoryService,
	services.NewPipelineStore,
)
