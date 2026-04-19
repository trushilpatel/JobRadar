package job

import (
	"github.com/google/wire"
	svc "github.com/trushilpatel/jobradar/service/job/services"
)

// ProviderSet is the Wire provider set for the job domain.
var ProviderSet = wire.NewSet(
	svc.NewHandler,
	svc.NewValidator,
	svc.NewService,
	svc.NewJobStore,
	svc.NewSeenLedgerStore,
	svc.NewDiscardStore,
	svc.NewSnapshotStore,
	svc.NewTechTagStore,
)
