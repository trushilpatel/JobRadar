package dashboard

import "github.com/google/wire"

// ProviderSet is the Wire provider set for the dashboard domain.
var ProviderSet = wire.NewSet(NewHandler, NewValidator, NewService)
