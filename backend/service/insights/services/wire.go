package services

import "github.com/google/wire"

// ProviderSet is the Wire provider set for the insights domain.
var ProviderSet = wire.NewSet(
	NewRestService,
	NewValidationService,
	NewImplService,
	NewRepositoryService,
)
