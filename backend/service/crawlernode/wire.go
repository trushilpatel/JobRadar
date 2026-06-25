package crawlernode

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// ProviderSet is the Wire provider set for the crawlernode domain.
//
// Chain:
//
//	NodeRepositoryService      (NodeStore — terminal)
//	AssignmentRepositoryService (AssignmentStore — terminal)
//	LedgerRepositoryService    (LedgerStore — terminal, wraps job.LedgerService)
//	ScanLogRepositoryService   (ScanLogStore — terminal, wraps scan.ScanLogWriter)
//	ScanSettingsReader         (ScanSettingsReader — terminal)
//	  → ImplService             (NodeService — business logic)
//	    → ValidationService     (NodeService — input validation)
//	      → NodeRestService     (HTTP handler — node-facing endpoints)
//	      → AdminRestService    (HTTP handler — admin endpoints)
//	        → NodeRestRouter    (route registration)
//	        → AdminRestRouter   (route registration)
//
// Note: NewImplService takes a tokenSecret string parameter. Wire callers
// must bind this via a named type provider or wire.Value at the app layer.
var ProviderSet = wire.NewSet(
	services.NewNodeRepositoryService,
	services.NewAssignmentRepositoryService,
	services.NewLedgerRepositoryService,
	services.NewScanLogRepositoryService,
	services.NewScanSettingsReader,
	services.NewImplService,
	services.NewValidationService,
	services.NewNodeRestService,
	services.NewAdminRestService,
	services.NewNodeRestRouter,
	services.NewAdminRestRouter,
)
