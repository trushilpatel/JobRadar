package application

import "github.com/google/wire"

// ProviderSet is the Wire provider set for the application domain.
var ProviderSet = wire.NewSet(NewHandler, NewValidator, NewService, NewStore)
