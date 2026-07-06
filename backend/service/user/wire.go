// Package user is the feature-slice root for the user domain. The wire
// provider set here composes the decorator chain in the canonical order:
//
//	RestRouter → RestService → Validation → Cache → Impl → Repository
//
// Wire builds the chain from the constructor signatures regardless of listed
// order — each constructor returns the same Service interface so composition
// is straightforward. The /api/me middleware chain is applied at construction
// time of UserRestRouter via the variadic middleware argument.
package user

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/user/services"
)

// ProviderSet is the Wire provider set for the user domain.
var ProviderSet = wire.NewSet(
	services.NewRestRouter,
	services.NewRestService,
	services.NewValidationService,
	services.NewCacheService,
	services.NewImplService,
	services.NewRepositoryService,
)
