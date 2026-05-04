package job

import (
	"github.com/google/wire"
	svc "github.com/trushilpatel/jobradar/service/job/services"
)

// ProviderSet is the Wire provider set for the job domain. The decorator
// chain is composed RestRouter → RestService → ValidationService →
// ImplService (terminal, holds JobReader + UserJobStateRepository + cache).
var ProviderSet = wire.NewSet(
	svc.NewRestRouter,
	svc.NewRestService,
	svc.NewValidationService,
	svc.NewImplService,
	svc.NewJobStore,
	svc.NewUserJobStateRepository,
	svc.NewSeenLedgerStore,
	svc.NewLedgerService,
	svc.NewSnapshotStore,
	svc.NewTechTagStore,
	svc.NewPipelineService,
)
