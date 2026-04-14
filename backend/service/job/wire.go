package job

import "github.com/google/wire"

// ProviderSet is the Wire provider set for the job domain.
var ProviderSet = wire.NewSet(NewHandler, NewValidator, NewService, NewJobStore, NewSeenLedgerStore, NewDiscardStore, NewSnapshotStore)
