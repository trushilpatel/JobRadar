package application

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/application/services"
)

// ProviderSet is the Wire provider set for the application domain.
//
// Chain (Phase 2):
//
//	RepositoryService (terminal ApplicationRepository)
//	  → ImplService (composes repo + jobsvc.JobService → ApplicationService)
//	    → ValidationService (wraps ApplicationService)
//	      → RestService (wraps ApplicationService)
//	        → RestRouter (wraps RestService)
//
// The RepositoryService now returns ApplicationRepository (per-user, takes
// userID explicitly) — the terminal ApplicationService implementation is
// the ImplService, which derives userID from ctx.
var ProviderSet = wire.NewSet(
	services.NewRestRouter,
	services.NewRestService,
	services.NewValidationService,
	services.NewImplService,
	services.NewRepositoryService,
)
