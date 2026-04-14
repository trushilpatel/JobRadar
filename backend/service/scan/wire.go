package scan

import "github.com/google/wire"

// ProviderSet is the Wire provider set for the scan domain.
var ProviderSet = wire.NewSet(NewHandler, NewValidator, NewService, NewQueueStore, NewLogStore)
