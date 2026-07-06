// Package ats is the feature-slice root for the ats domain. The wire
// provider set here composes the decorator chain in the canonical order:
//
//	RestRouter → RestService → Validation → Cache → Impl → Repository
//
// Wire builds the chain from the constructor signatures regardless of
// listed order. Each constructor returns the same AtsService interface so
// composition is straightforward.
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
