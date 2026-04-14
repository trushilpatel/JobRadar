package company

import "github.com/google/wire"

// ProviderSet is the Wire provider set for the company domain.
var ProviderSet = wire.NewSet(NewHandler, NewValidator, NewService, NewStore)
