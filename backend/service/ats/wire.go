// Package ats is the feature-slice root for the ats domain. The wire
// provider set here documents the intended composition, in canonical order:
//
//	RestRouter → RestService → Validation → Cache → Impl → Repository
//
// NOTE: this ProviderSet is never fed through Wire's codegen — Wire can't
// auto-resolve a linear chain of providers that all return the same
// AtsService interface (that's "multiple bindings", not a chain to Wire).
// See apps/api/CLAUDE.md; apps/api/wire_gen.go is hand-maintained.
package ats

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/ats/services"
)

// ProviderSet is the Wire provider set for the ats domain.
var ProviderSet = wire.NewSet(
	services.NewRestRouter,
	services.NewRestService,
	services.NewValidationService,
	services.NewCacheService,
	services.NewImplService,
	services.NewRepositoryService,
)
