// Package ats is the feature-slice root for the ats domain. The wire
// provider set here composes the decorator chain in the canonical order:
//
//	Repository → Impl → Cache → Validation → Rest → RestRouter
//
// Wire builds the chain right-to-left from the constructor signatures.
// Each constructor returns the same AtsService interface so composition
// is straightforward.
package ats

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/ats/services"
)

// ProviderSet is the Wire provider set for the ats domain.
var ProviderSet = wire.NewSet(
	services.NewAtsRepositoryService,
	services.NewAtsImplService,
	services.NewAtsCacheService,
	services.NewAtsValidationService,
	services.NewAtsRestService,
	services.NewAtsRestRouter,
)
