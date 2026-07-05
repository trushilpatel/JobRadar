package applyqueue

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/applyqueue/services"
)

// ProviderSet is the Wire provider set for the apply-queue domain. The
// decorator chain is composed RestRouter → RestService → ValidationService →
// ImplService → RepositoryService (terminal). ValidationService additionally
// depends on EngagementChecker and ResumeOwnershipChecker, each backed by its
// own constructor over the same stateless repositoryService struct — see
// services/ApplyQueueService.go for why three separate constructors exist
// instead of one combined interface.
var ProviderSet = wire.NewSet(
	services.NewRestRouter,
	services.NewRestService,
	services.NewValidationService,
	services.NewImplService,
	services.NewRepositoryService,
	services.NewEngagementChecker,
	services.NewResumeOwnershipChecker,
)
