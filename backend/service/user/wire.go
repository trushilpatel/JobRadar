// Package user is the feature-slice root for the user domain. The wire
// provider set here composes the decorator chain in the canonical order:
//
//	Repository → Impl → Cache → Validation → Rest → RestRouter
//
// Wire builds the chain right-to-left from the constructor signatures —
// each constructor returns the same Service interface so composition is
// straightforward. The /api/me middleware chain is applied at construction
// time of UserRestRouter via the variadic middleware argument.
package user

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/user/services"
)

// ProviderSet is the Wire provider set for the user domain.
var ProviderSet = wire.NewSet(
	services.NewRepositoryService,
	services.NewImplService,
	services.NewCacheService,
	services.NewValidationService,
	services.NewRestService,
	services.NewRestRouter,
)
